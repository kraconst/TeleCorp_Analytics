package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

const (
	SyncTimeout            = 10 * time.Minute
	syncAccountListTimeout = 15 * time.Second
	chatUpdateQueueSize    = 128
)

type SyncResult struct {
	ChatsProcessed       int `json:"chats_processed"`
	NewMessages          int `json:"new_messages"`
	NotificationsDropped int `json:"notifications_dropped,omitempty"`
}

type SyncOptions struct {
	OnChatUpdated func(ctx context.Context, companyID, chatID int64)
}

type chatUpdate struct {
	companyID int64
	chatID    int64
}

type SyncUsecase struct {
	repo          SyncRepository
	telegram      TelegramSyncClient
	onChatUpdated func(context.Context, int64, int64)
	updates       chan chatUpdate
	mu            sync.Mutex
	running       map[SyncAccountRef]struct{}
}

func NewSyncUsecase(repo SyncRepository, telegram TelegramSyncClient, options SyncOptions) *SyncUsecase {
	u := &SyncUsecase{
		repo: repo, telegram: telegram, onChatUpdated: options.OnChatUpdated,
		running: make(map[SyncAccountRef]struct{}),
	}
	if u.onChatUpdated != nil {
		u.updates = make(chan chatUpdate, chatUpdateQueueSize)
	}
	return u
}

// SyncAccount reports the committed progress even when a later dialog fails.
func (u *SyncUsecase) SyncAccount(ctx context.Context, companyID, accountID int64) (SyncResult, error) {
	var result SyncResult
	if companyID <= 0 || accountID <= 0 {
		return result, fmt.Errorf("%w: company and account IDs must be positive", domain.ErrValidation)
	}
	if u == nil || u.telegram == nil || u.repo == nil {
		return result, ErrTelegramUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, SyncTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	ref := SyncAccountRef{CompanyID: companyID, AccountID: accountID}
	if !u.claim(ref) {
		return result, fmt.Errorf("%w: account sync is already running", domain.ErrConflict)
	}
	defer u.release(ref)

	status, err := u.repo.SyncAccountStatus(ctx, companyID, accountID)
	if err != nil {
		return result, fmt.Errorf("get account for sync: %w", err)
	}
	if status != domain.StatusActive {
		return result, fmt.Errorf("%w: account must be active to sync", domain.ErrConflict)
	}

	err = u.telegram.WithAccount(ctx, companyID, accountID, func(ctx context.Context, reader TelegramAccountReader) error {
		dialogs, err := reader.ListDialogs(ctx)
		if err != nil {
			return fmt.Errorf("list dialogs for sync: %w", err)
		}
		for _, dialog := range dialogs {
			if err := ctx.Err(); err != nil {
				return err
			}
			imported, err := u.syncDialog(ctx, ref, reader, dialog)
			if err != nil {
				return err
			}
			result.ChatsProcessed++
			result.NewMessages += imported.NewMessages
			if imported.NewMessages > 0 && u.updates != nil {
				select {
				case u.updates <- chatUpdate{companyID: companyID, chatID: imported.ChatID}:
				default:
					result.NotificationsDropped++
				}
			}
		}
		return nil
	})
	return result, err
}

func (u *SyncUsecase) syncDialog(ctx context.Context, ref SyncAccountRef, reader TelegramAccountReader, dialog TelegramDialog) (SyncDialogResult, error) {
	if dialog.TelegramUserID <= 0 || dialog.UnreadCount < 0 {
		return SyncDialogResult{}, fmt.Errorf("invalid dialog metadata from Telegram")
	}
	checkpoint, err := u.repo.DialogCheckpoint(ctx, ref.CompanyID, ref.AccountID, dialog.TelegramUserID)
	if err != nil {
		return SyncDialogResult{}, fmt.Errorf("read message checkpoint: %w", err)
	}
	history, err := reader.ListHistory(ctx, dialog, checkpoint)
	if err != nil {
		return SyncDialogResult{}, fmt.Errorf("read history for sync: %w", err)
	}
	newMessages := make([]domain.Message, 0, len(history))
	for _, message := range history {
		if message.TelegramMessageID <= checkpoint {
			continue
		}
		if message.SentAt.IsZero() || (message.Direction != domain.MessageDirectionIn && message.Direction != domain.MessageDirectionOut) {
			return SyncDialogResult{}, fmt.Errorf("invalid message metadata from Telegram")
		}
		newMessages = append(newMessages, message)
	}
	imported, err := u.repo.ImportDialog(ctx, ref.CompanyID, ref.AccountID, dialog, newMessages)
	if err != nil {
		return SyncDialogResult{}, fmt.Errorf("import Telegram dialog: %w", err)
	}
	return imported, nil
}

func (u *SyncUsecase) claim(ref SyncAccountRef) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, exists := u.running[ref]; exists {
		return false
	}
	u.running[ref] = struct{}{}
	return true
}

func (u *SyncUsecase) release(ref SyncAccountRef) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.running, ref)
}

// RunNotifications dispatches post-commit events serially on the application
// context. The callback must honor cancellation during shutdown.
func (u *SyncUsecase) RunNotifications(ctx context.Context) {
	if u.updates == nil {
		return
	}
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case update := <-u.updates:
			u.onChatUpdated(ctx, update.companyID, update.chatID)
		}
	}
}

// RunWorker runs an initial pass immediately and later passes on the ticker.
// A zero account reference in report denotes failure to list eligible accounts.
func (u *SyncUsecase) RunWorker(ctx context.Context, interval time.Duration, report func(SyncAccountRef, SyncResult, error)) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		u.syncAccounts(ctx, report)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (u *SyncUsecase) syncAccounts(ctx context.Context, report func(SyncAccountRef, SyncResult, error)) {
	listCtx, cancel := context.WithTimeout(ctx, syncAccountListTimeout)
	accounts, err := u.repo.ListSyncAccounts(listCtx)
	cancel()
	if err != nil {
		if report != nil && ctx.Err() == nil {
			report(SyncAccountRef{}, SyncResult{}, err)
		}
		return
	}
	for _, account := range accounts {
		if ctx.Err() != nil {
			return
		}
		result, err := u.SyncAccount(ctx, account.CompanyID, account.AccountID)
		if report != nil && ctx.Err() == nil {
			report(account, result, err)
		}
	}
}
