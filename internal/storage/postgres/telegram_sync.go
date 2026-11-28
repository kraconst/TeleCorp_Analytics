package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/jmoiron/sqlx"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type TelegramSyncStorage struct {
	db *sqlx.DB
}

var _ usecase.SyncRepository = (*TelegramSyncStorage)(nil)

func NewTelegramSyncStorage(db *sqlx.DB) *TelegramSyncStorage {
	return &TelegramSyncStorage{db: db}
}

type syncAccountRow struct {
	CompanyID int64 `db:"company_id"`
	AccountID int64 `db:"id"`
}

func (s *TelegramSyncStorage) ListSyncAccounts(ctx context.Context) ([]usecase.SyncAccountRef, error) {
	const query = `
		SELECT company_id, id
		FROM accounts
		WHERE status = $1 AND octet_length(session) > 0
		ORDER BY company_id, id
	`
	var rows []syncAccountRow
	if err := s.db.SelectContext(ctx, &rows, query, domain.StatusActive); err != nil {
		return nil, mapPgError(fmt.Errorf("list Telegram sync accounts: %w", err))
	}
	accounts := make([]usecase.SyncAccountRef, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, usecase.SyncAccountRef{
			CompanyID: row.CompanyID,
			AccountID: row.AccountID,
		})
	}
	return accounts, nil
}

func (s *TelegramSyncStorage) SyncAccountStatus(ctx context.Context, companyID, accountID int64) (domain.AccountStatus, error) {
	const query = `SELECT status FROM accounts WHERE company_id = $1 AND id = $2`
	var status domain.AccountStatus
	if err := s.db.GetContext(ctx, &status, query, companyID, accountID); err != nil {
		return "", mapPgError(fmt.Errorf("read Telegram sync account status: %w", err))
	}
	return status, nil
}

func (s *TelegramSyncStorage) DialogCheckpoint(ctx context.Context, companyID, accountID, telegramUserID int64) (int64, error) {
	const query = `
		SELECT COALESCE((
			SELECT MAX(m.telegram_message_id)
			FROM contacts AS contact
			JOIN chats AS chat ON chat.contact_id = contact.id AND chat.account_id = account.id
			JOIN messages AS m ON m.chat_id = chat.id
			WHERE contact.company_id = account.company_id AND contact.telegram_user_id = $3
		), 0)
		FROM accounts AS account
		WHERE account.company_id = $1 AND account.id = $2
	`
	var checkpoint int64
	if err := s.db.GetContext(ctx, &checkpoint, query, companyID, accountID, telegramUserID); err != nil {
		return 0, mapPgError(fmt.Errorf("read Telegram dialog checkpoint: %w", err))
	}
	return checkpoint, nil
}

func (s *TelegramSyncStorage) ImportDialog(ctx context.Context, companyID, accountID int64, dialog usecase.TelegramDialog, messages []domain.Message) (usecase.SyncDialogResult, error) {
	var result usecase.SyncDialogResult
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin Telegram dialog import: %w", err)
	}
	defer tx.Rollback()

	location, err := lockSyncAccount(ctx, tx, companyID, accountID)
	if err != nil {
		return result, err
	}
	contactID, newContact, err := importTelegramContact(ctx, tx, companyID, dialog)
	if err != nil {
		return result, err
	}
	chatID, err := importTelegramChat(ctx, tx, accountID, contactID)
	if err != nil {
		return result, err
	}
	inserted, last, err := importTelegramMessages(ctx, tx, accountID, chatID, location, newContact, messages)
	if err != nil {
		return result, err
	}
	if err := updateTelegramChat(ctx, tx, chatID, dialog.UnreadCount, inserted > 0); err != nil {
		return result, err
	}
	if inserted > 0 {
		const query = `UPDATE accounts SET last_activity_at = GREATEST(last_activity_at, $3) WHERE company_id = $1 AND id = $2`
		if _, err := tx.ExecContext(ctx, query, companyID, accountID, last); err != nil {
			return result, mapPgError(fmt.Errorf("update Telegram account activity: %w", err))
		}
	}
	if err := tx.Commit(); err != nil {
		return result, mapPgError(fmt.Errorf("commit Telegram dialog import: %w", err))
	}
	return usecase.SyncDialogResult{ChatID: chatID, NewMessages: inserted}, nil
}

func lockSyncAccount(ctx context.Context, tx *sqlx.Tx, companyID, accountID int64) (*time.Location, error) {
	const query = `
		SELECT account.status, company.timezone
		FROM accounts AS account
		JOIN companies AS company ON company.id = account.company_id
		WHERE account.company_id = $1 AND account.id = $2
		FOR UPDATE OF account
	`
	var status domain.AccountStatus
	var timezone string
	if err := tx.QueryRowxContext(ctx, query, companyID, accountID).Scan(&status, &timezone); err != nil {
		return nil, mapPgError(fmt.Errorf("lock Telegram sync account: %w", err))
	}
	if status != domain.StatusActive {
		return nil, fmt.Errorf("%w: Telegram account is not active", domain.ErrConflict)
	}
	location, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" || timezone == "Local" {
		return nil, fmt.Errorf("%w: invalid company timezone", domain.ErrValidation)
	}
	return location, nil
}

func importTelegramContact(ctx context.Context, tx *sqlx.Tx, companyID int64, dialog usecase.TelegramDialog) (int64, bool, error) {
	const insert = `
		INSERT INTO contacts (company_id, telegram_user_id, name, username)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (company_id, telegram_user_id) DO NOTHING
		RETURNING id
	`
	var id int64
	err := tx.QueryRowxContext(ctx, insert, companyID, dialog.TelegramUserID, dialog.Name, dialog.Username).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, mapPgError(fmt.Errorf("insert Telegram contact: %w", err))
	}
	const update = `
		UPDATE contacts SET name = $3, username = $4
		WHERE company_id = $1 AND telegram_user_id = $2
		RETURNING id
	`
	if err := tx.QueryRowxContext(ctx, update, companyID, dialog.TelegramUserID, dialog.Name, dialog.Username).Scan(&id); err != nil {
		return 0, false, mapPgError(fmt.Errorf("refresh Telegram contact: %w", err))
	}
	return id, false, nil
}

func importTelegramChat(ctx context.Context, tx *sqlx.Tx, accountID, contactID int64) (int64, error) {
	const insert = `
		INSERT INTO chats (account_id, contact_id)
		VALUES ($1, $2)
		ON CONFLICT (account_id, contact_id) DO NOTHING
		RETURNING id
	`
	var id int64
	err := tx.GetContext(ctx, &id, insert, accountID, contactID)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, mapPgError(fmt.Errorf("insert Telegram chat: %w", err))
	}
	const selectChat = `SELECT id FROM chats WHERE account_id = $1 AND contact_id = $2 FOR UPDATE`
	if err := tx.GetContext(ctx, &id, selectChat, accountID, contactID); err != nil {
		return 0, mapPgError(fmt.Errorf("lock Telegram chat: %w", err))
	}
	return id, nil
}

func importTelegramMessages(ctx context.Context, tx *sqlx.Tx, accountID, chatID int64, location *time.Location, newContact bool, messages []domain.Message) (int, time.Time, error) {
	var first, last time.Time
	inserted := 0
	daily := make(map[string]*domain.AccountDailyStats)
	for _, msg := range messages {
		msg.ChatID = chatID
		id, err := insertMessage(ctx, tx, msg)
		if err != nil {
			return 0, time.Time{}, err
		}
		if id == 0 {
			continue
		}
		inserted++
		if first.IsZero() || msg.SentAt.Before(first) {
			first = msg.SentAt
		}
		if last.IsZero() || msg.SentAt.After(last) {
			last = msg.SentAt
		}
		stats := syncDayStats(daily, msg.SentAt.In(location))
		if msg.Direction == domain.MessageDirectionIn {
			stats.MessagesIn++
		} else {
			stats.MessagesOut++
		}
	}
	if newContact {
		if first.IsZero() {
			first = time.Now()
		}
		syncDayStats(daily, first.In(location)).NewContacts++
	}
	days := make([]string, 0, len(daily))
	for day := range daily {
		days = append(days, day)
	}
	sort.Strings(days)
	for _, day := range days {
		stats := daily[day]
		if err := incrementAccountDailyStats(ctx, tx, accountID, stats.Day, stats.MessagesIn, stats.MessagesOut, stats.NewContacts); err != nil {
			return 0, time.Time{}, err
		}
	}
	if len(days) > 0 {
		if err := refreshAccountDailyMetrics(ctx, tx, accountID, days, location); err != nil {
			return 0, time.Time{}, err
		}
	}
	return inserted, last, nil
}

func refreshAccountDailyMetrics(ctx context.Context, tx *sqlx.Tx, accountID int64, days []string, location *time.Location) error {
	minDay, err := time.ParseInLocation(time.DateOnly, days[0], location)
	if err != nil {
		return fmt.Errorf("parse min stats day: %w", err)
	}
	maxDay, err := time.ParseInLocation(time.DateOnly, days[len(days)-1], location)
	if err != nil {
		return fmt.Errorf("parse max stats day: %w", err)
	}
	// Bound lower scan window to 30 days before the earliest affected day so cross-day bursts are captured
	// without scanning years of historical messages on every sync.
	windowStart := minDay.AddDate(0, 0, -30).UTC()
	windowEnd := maxDay.AddDate(0, 0, 1).UTC()
	const query = `
		WITH chat_msgs AS (
			SELECT
				m.chat_id,
				m.direction,
				m.sent_at,
				(m.sent_at AT TIME ZONE $4)::date AS local_day,
				LAG(m.direction) OVER (PARTITION BY m.chat_id ORDER BY m.sent_at ASC, m.telegram_message_id ASC) AS prev_direction,
				COUNT(*) FILTER (WHERE m.direction = 'out') OVER (
					PARTITION BY m.chat_id
					ORDER BY m.sent_at ASC, m.telegram_message_id ASC
					ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
				) AS out_before
			FROM messages m
			JOIN chats c ON c.id = m.chat_id
			WHERE c.account_id = $1
			  AND m.sent_at >= $2
			  AND m.sent_at < $3
		), burst_starts AS (
			SELECT
				chat_id,
				out_before,
				MIN(sent_at) AS first_in_at
			FROM chat_msgs
			WHERE direction = 'in'
			GROUP BY chat_id, out_before
		), day_metrics AS (
			SELECT
				cm.local_day AS day,
				COUNT(DISTINCT cm.chat_id) AS active_chats,
				COALESCE(
					AVG(EXTRACT(EPOCH FROM (cm.sent_at - bs.first_in_at)) * 1000000000.0)
					FILTER (
						WHERE cm.direction = 'out'
						  AND cm.prev_direction = 'in'
						  AND bs.first_in_at IS NOT NULL
						  AND cm.sent_at >= bs.first_in_at
					),
					0
				)::BIGINT AS avg_response_time
			FROM chat_msgs cm
			LEFT JOIN burst_starts bs
			  ON bs.chat_id = cm.chat_id AND bs.out_before = cm.out_before
			WHERE cm.local_day = ANY(string_to_array($5, ',')::date[])
			GROUP BY cm.local_day
		)
		UPDATE account_daily_stats ads
		SET active_chats = dm.active_chats,
		    avg_response_time = dm.avg_response_time
		FROM day_metrics dm
		WHERE ads.account_id = $1 AND ads.day = dm.day
	`
	if _, err := tx.ExecContext(ctx, query, accountID, windowStart, windowEnd, location.String(), strings.Join(days, ",")); err != nil {
		return mapPgError(fmt.Errorf("refresh account daily metrics: %w", err))
	}
	return nil
}

func syncDayStats(daily map[string]*domain.AccountDailyStats, localTime time.Time) *domain.AccountDailyStats {
	day := localTime.Format(time.DateOnly)
	stats := daily[day]
	if stats == nil {
		stats = &domain.AccountDailyStats{Day: localTime}
		daily[day] = stats
	}
	return stats
}

func updateTelegramChat(ctx context.Context, tx *sqlx.Tx, chatID int64, unreadCount int, historyChanged bool) error {
	query := `UPDATE chats SET unread_count = $2 WHERE id = $1`
	if historyChanged {
		query = `
			WITH last_outgoing AS (
				SELECT sent_at, telegram_message_id
				FROM messages
				WHERE chat_id = $1 AND direction = 'out'
				ORDER BY sent_at DESC, telegram_message_id DESC
				LIMIT 1
			), history AS (
				SELECT MAX(m.sent_at) AS latest,
					MIN(m.sent_at) FILTER (WHERE m.direction = 'in' AND
						(o.telegram_message_id IS NULL OR (m.sent_at, m.telegram_message_id) > (o.sent_at, o.telegram_message_id))) AS awaiting
				FROM messages AS m
				LEFT JOIN last_outgoing AS o ON true
				WHERE m.chat_id = $1
			)
			UPDATE chats SET unread_count = $2, last_message_at = history.latest, awaiting_reply_since = history.awaiting
			FROM history WHERE chats.id = $1
		`
	}
	if _, err := tx.ExecContext(ctx, query, chatID, unreadCount); err != nil {
		return mapPgError(fmt.Errorf("update Telegram chat metadata: %w", err))
	}
	return nil
}
