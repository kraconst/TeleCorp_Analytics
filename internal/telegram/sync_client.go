package telegram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/session"
	gotd "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/kraconst/GO_server/internal/config"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

const (
	syncRPCTimeout = 15 * time.Second
	syncTimeout    = 10 * time.Minute
)

// SyncClient opens same-account Telegram readers using existing authorized sessions.
type SyncClient struct {
	apiID    int
	apiHash  string
	key      []byte
	limit    int
	resolver dcs.Resolver
	accounts usecase.TelegramAccountStateRepository
}

var _ usecase.TelegramSyncClient = (*SyncClient)(nil)

func NewSyncClient(cfg config.TelegramConfig, accounts usecase.TelegramAccountStateRepository) (*SyncClient, error) {
	if !cfg.Configured() {
		return nil, usecase.ErrTelegramUnavailable
	}
	if accounts == nil || cfg.SyncBatchLimit < 1 || cfg.SyncBatchLimit > 100 {
		return nil, fmt.Errorf("%w: Telegram sync needs a repository and batch size in 1–100", domain.ErrValidation)
	}
	var resolver dcs.Resolver
	if address := strings.TrimSpace(cfg.Proxy); address != "" {
		dialer, err := createProxyDialer(address)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid Telegram proxy configuration", domain.ErrValidation)
		}
		resolver = dcs.Plain(dcs.PlainOptions{Dial: dialer})
	}
	return &SyncClient{
		apiID: cfg.APIID, apiHash: strings.TrimSpace(cfg.APIHash), key: bytes.Clone(cfg.SessionKey),
		limit: cfg.SyncBatchLimit, resolver: resolver, accounts: accounts,
	}, nil
}

// WithAccount restores an authorized session and passes its reader to fn.
// The caller serializes sync runs and session changes for an account.
func (s *SyncClient) WithAccount(ctx context.Context, companyID, accountID int64, fn func(context.Context, usecase.TelegramAccountReader) error) error {
	if fn == nil {
		return fmt.Errorf("%w: Telegram sync callback is required", domain.ErrValidation)
	}
	storage, err := NewAccountSessionStorage(s.accounts, companyID, accountID, s.key)
	if err != nil {
		return err
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, syncTimeout)
		defer cancel()
	}
	raw, err := storage.LoadSession(ctx)
	if errors.Is(err, session.ErrNotFound) {
		return s.recordSessionFailure(ctx, companyID, accountID, domain.StatusDisconnected, err)
	}
	if err != nil {
		return err
	}
	// gotd may save a freshly created auth key before Self returns. Buffer all
	// writes until this run verified the session and completed successfully.
	buffer := &session.StorageMemory{}
	if err := buffer.StoreSession(ctx, raw); err != nil {
		return fmt.Errorf("buffer Telegram session: %w", err)
	}
	loader := &session.Loader{Storage: buffer}
	key, err := loadSavedAuthKey(ctx, loader)
	if err != nil {
		return err
	}
	client := gotd.NewClient(s.apiID, s.apiHash, gotd.Options{
		SessionStorage: buffer,
		Resolver:       s.resolver,
		NoUpdates:      true,
		DialTimeout:    syncRPCTimeout,
	})
	var completed bool
	err = client.Run(ctx, func(runCtx context.Context) error {
		self, err := retryTelegramRPC(runCtx, client.Self)
		if err != nil {
			return fmt.Errorf("verify Telegram authorization: %w", err)
		}
		if self.ID <= 0 || self.Bot || self.Deleted {
			return usecase.ErrTelegramSessionInvalid
		}
		if err := verifySavedAuthKey(runCtx, loader, key); err != nil {
			return err
		}
		reader := &accountReader{api: client.API(), limit: s.limit, selfID: self.ID}
		if err := fn(runCtx, reader); err != nil {
			return err
		}
		completed = true
		return nil
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		if status := rejectedSessionStatus(err); status != "" {
			return s.recordSessionFailure(ctx, companyID, accountID, status, err)
		}
		return fmt.Errorf("run Telegram sync: %w", err)
	}
	if !completed {
		return errors.New("Telegram connection closed before sync completed")
	}
	if err := verifySavedAuthKey(ctx, loader, key); err != nil {
		if errors.Is(err, usecase.ErrTelegramSessionInvalid) {
			return s.recordSessionFailure(ctx, companyID, accountID, domain.StatusDisconnected, err)
		}
		return err
	}
	data, err := buffer.LoadSession(ctx)
	if err != nil {
		return fmt.Errorf("read updated Telegram session: %w", err)
	}
	if bytes.Equal(data, raw) {
		return nil
	}
	return storage.StoreSession(ctx, data)
}

func verifySavedAuthKey(ctx context.Context, loader *session.Loader, original crypto.Key) error {
	key, err := loadSavedAuthKey(ctx, loader)
	if err != nil {
		return err
	}
	if key != original {
		return fmt.Errorf("%w: Telegram replaced the saved authorization key", usecase.ErrTelegramSessionInvalid)
	}
	return nil
}

func loadSavedAuthKey(ctx context.Context, loader *session.Loader) (crypto.Key, error) {
	data, err := loader.Load(ctx)
	if err != nil {
		return crypto.Key{}, errUnreadableAccountSession
	}
	var key crypto.Key
	if len(data.AuthKey) != len(key) {
		return crypto.Key{}, errUnreadableAccountSession
	}
	copy(key[:], data.AuthKey)
	keyID := key.ID()
	if key.Zero() || !bytes.Equal(keyID[:], data.AuthKeyID) {
		return crypto.Key{}, errUnreadableAccountSession
	}
	return key, nil
}

func (s *SyncClient) recordSessionFailure(ctx context.Context, companyID, accountID int64, status domain.AccountStatus, cause error) error {
	failure := fmt.Errorf("%w: %w", usecase.ErrTelegramSessionInvalid, cause)
	if err := s.accounts.UpdateSessionStatus(ctx, companyID, accountID, status); err != nil {
		return errors.Join(failure, fmt.Errorf("update Telegram session status: %w", err))
	}
	return failure
}

func rejectedSessionStatus(err error) domain.AccountStatus {
	switch {
	case tgerr.Is(err, "USER_DEACTIVATED_BAN", "PHONE_NUMBER_BANNED"):
		return domain.StatusBanned
	case errors.Is(err, usecase.ErrTelegramSessionInvalid),
		tgerr.Is(err, "SESSION_REVOKED", "SESSION_EXPIRED", "AUTH_KEY_UNREGISTERED", "AUTH_KEY_INVALID", "AUTH_KEY_DUPLICATED", "USER_DEACTIVATED"):
		return domain.StatusDisconnected
	default:
		return ""
	}
}

func retryTelegramRPC[T any](ctx context.Context, call func(context.Context) (T, error)) (T, error) {
	var zero T
	for ctx.Err() == nil {
		attemptCtx, cancel := context.WithTimeout(ctx, syncRPCTimeout)
		result, err := call(attemptCtx)
		cancel()
		wait, limited := tgerr.AsFloodWait(err)
		if !limited {
			return result, err
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}
	return zero, ctx.Err()
}

type syncRPC interface {
	MessagesGetPinnedDialogs(context.Context, int) (*tg.MessagesPeerDialogs, error)
	MessagesGetDialogs(context.Context, *tg.MessagesGetDialogsRequest) (tg.MessagesDialogsClass, error)
	MessagesGetHistory(context.Context, *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error)
}
