package usecase_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type syncRepositoryStub struct {
	list       func(context.Context) ([]usecase.SyncAccountRef, error)
	status     func(context.Context, int64, int64) (domain.AccountStatus, error)
	checkpoint func(context.Context, int64, int64, int64) (int64, error)
	importChat func(context.Context, int64, int64, usecase.TelegramDialog, []domain.Message) (usecase.SyncDialogResult, error)
}

func (s syncRepositoryStub) ListSyncAccounts(ctx context.Context) ([]usecase.SyncAccountRef, error) {
	return s.list(ctx)
}

func (s syncRepositoryStub) SyncAccountStatus(ctx context.Context, companyID, accountID int64) (domain.AccountStatus, error) {
	return s.status(ctx, companyID, accountID)
}

func (s syncRepositoryStub) DialogCheckpoint(ctx context.Context, companyID, accountID, telegramUserID int64) (int64, error) {
	return s.checkpoint(ctx, companyID, accountID, telegramUserID)
}

func (s syncRepositoryStub) ImportDialog(ctx context.Context, companyID, accountID int64, dialog usecase.TelegramDialog, messages []domain.Message) (usecase.SyncDialogResult, error) {
	return s.importChat(ctx, companyID, accountID, dialog, messages)
}

type syncClientFunc func(context.Context, int64, int64, func(context.Context, usecase.TelegramAccountReader) error) error

func (f syncClientFunc) WithAccount(ctx context.Context, companyID, accountID int64, fn func(context.Context, usecase.TelegramAccountReader) error) error {
	return f(ctx, companyID, accountID, fn)
}

type telegramHistoryStub struct {
	dialogs []usecase.TelegramDialog
	history func(usecase.TelegramDialog, int64) ([]domain.Message, error)
}

func (s telegramHistoryStub) ListDialogs(context.Context) ([]usecase.TelegramDialog, error) {
	return s.dialogs, nil
}

func (s telegramHistoryStub) ListHistory(_ context.Context, dialog usecase.TelegramDialog, afterID int64) ([]domain.Message, error) {
	return s.history(dialog, afterID)
}

func activeForSync(context.Context, int64, int64) (domain.AccountStatus, error) {
	return domain.StatusActive, nil
}

func TestSyncAccountContinuesFromCommittedCheckpointAndEnqueuesOnlyNewMessages(t *testing.T) {
	const companyID, accountID, telegramUserID, chatID = 8, 10, 15, 21
	checkpoint := int64(4)
	var offsets []int64
	var unreadUpdates int
	now := time.Now()
	reader := telegramHistoryStub{
		dialogs: []usecase.TelegramDialog{{TelegramUserID: telegramUserID, UnreadCount: 2}},
		history: func(_ usecase.TelegramDialog, afterID int64) ([]domain.Message, error) {
			offsets = append(offsets, afterID)
			return []domain.Message{
				{TelegramMessageID: 4, SentAt: now, Direction: domain.MessageDirectionIn},
				{TelegramMessageID: 5, SentAt: now, Direction: domain.MessageDirectionOut},
			}, nil
		},
	}
	store := syncRepositoryStub{
		status: activeForSync,
		checkpoint: func(_ context.Context, company, account, peer int64) (int64, error) {
			if company != companyID || account != accountID || peer != telegramUserID {
				t.Fatalf("wrong scope for checkpoint: %d %d %d", company, account, peer)
			}
			return checkpoint, nil
		},
		importChat: func(_ context.Context, company, account int64, dialog usecase.TelegramDialog, items []domain.Message) (usecase.SyncDialogResult, error) {
			if company != companyID || account != accountID || dialog.UnreadCount != 2 {
				t.Fatal("incorrect import arguments")
			}
			unreadUpdates++
			if len(items) > 0 {
				if len(items) != 1 || items[0].TelegramMessageID != 5 {
					t.Fatalf("wrong new messages: %+v", items)
				}
				checkpoint = 5
			}
			return usecase.SyncDialogResult{ChatID: chatID, NewMessages: len(items)}, nil
		},
	}
	client := syncClientFunc(func(ctx context.Context, company, account int64, use func(context.Context, usecase.TelegramAccountReader) error) error {
		if company != companyID || account != accountID {
			t.Fatal("wrong scope for Telegram")
		}
		return use(ctx, reader)
	})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	var events [][2]int64
	u := usecase.NewSyncUsecase(store, client, usecase.SyncOptions{OnChatUpdated: func(_ context.Context, company, chat int64) {
		events = append(events, [2]int64{company, chat})
		cancel()
	}})
	for i, want := range []int{1, 0} {
		got, err := u.SyncAccount(ctx, companyID, accountID)
		if err != nil || got.ChatsProcessed != 1 || got.NewMessages != want {
			t.Fatalf("run %d returned %+v, %v", i, got, err)
		}
	}
	u.RunNotifications(ctx)
	if !reflect.DeepEqual(offsets, []int64{4, 5}) || unreadUpdates != 2 || !reflect.DeepEqual(events, [][2]int64{{companyID, chatID}}) {
		t.Fatalf("offsets=%v unread writes=%d events=%v", offsets, unreadUpdates, events)
	}
}

func TestSyncAccountReportsPartialCommitAndSessionError(t *testing.T) {
	store := syncRepositoryStub{
		status:     activeForSync,
		checkpoint: func(context.Context, int64, int64, int64) (int64, error) { return 0, nil },
		importChat: func(context.Context, int64, int64, usecase.TelegramDialog, []domain.Message) (usecase.SyncDialogResult, error) {
			return usecase.SyncDialogResult{ChatID: 2, NewMessages: 1}, nil
		},
	}
	reader := telegramHistoryStub{
		dialogs: []usecase.TelegramDialog{{TelegramUserID: 1}, {TelegramUserID: 2}},
		history: func(dialog usecase.TelegramDialog, _ int64) ([]domain.Message, error) {
			if dialog.TelegramUserID == 2 {
				return nil, usecase.ErrTelegramSessionInvalid
			}
			return []domain.Message{{TelegramMessageID: 1, Direction: domain.MessageDirectionIn, SentAt: time.Now()}}, nil
		},
	}
	u := usecase.NewSyncUsecase(store, syncClientFunc(func(ctx context.Context, _, _ int64, fn func(context.Context, usecase.TelegramAccountReader) error) error {
		return fn(ctx, reader)
	}), usecase.SyncOptions{})
	got, err := u.SyncAccount(t.Context(), 1, 1)
	if !errors.Is(err, usecase.ErrTelegramSessionInvalid) || got.ChatsProcessed != 1 || got.NewMessages != 1 {
		t.Fatalf("partial sync returned %+v, %v", got, err)
	}
}

func TestSyncAccountRejectsForeignAndInactiveAccountBeforeTelegram(t *testing.T) {
	store := syncRepositoryStub{status: func(_ context.Context, company, _ int64) (domain.AccountStatus, error) {
		if company != 3 {
			return "", domain.ErrNotFound
		}
		return domain.StatusDisconnected, nil
	}}
	u := usecase.NewSyncUsecase(store, syncClientFunc(func(context.Context, int64, int64, func(context.Context, usecase.TelegramAccountReader) error) error {
		t.Fatal("ineligible account reached Telegram")
		return nil
	}), usecase.SyncOptions{})
	for _, tc := range []struct {
		companyID int64
		want      error
	}{{0, domain.ErrValidation}, {2, domain.ErrNotFound}, {3, domain.ErrConflict}} {
		if _, err := u.SyncAccount(t.Context(), tc.companyID, 7); !errors.Is(err, tc.want) {
			t.Fatalf("company %d, got %v, want %v", tc.companyID, err, tc.want)
		}
	}
}

func TestSyncWorkerSkipsFailedAccountAndCancelsInFlight(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	stopped := make(chan struct{})
	results := make(chan error, 2)
	failure := errors.New("first account failed")
	store := syncRepositoryStub{
		status: activeForSync,
		list: func(context.Context) ([]usecase.SyncAccountRef, error) {
			return []usecase.SyncAccountRef{{CompanyID: 1, AccountID: 2}, {CompanyID: 2, AccountID: 3}}, nil
		},
	}
	client := syncClientFunc(func(ctx context.Context, _, account int64, _ func(context.Context, usecase.TelegramAccountReader) error) error {
		if account == 2 {
			return failure
		}
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	u := usecase.NewSyncUsecase(store, client, usecase.SyncOptions{})
	go func() {
		defer close(stopped)
		u.RunWorker(ctx, time.Hour, func(_ usecase.SyncAccountRef, _ usecase.SyncResult, err error) { results <- err })
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not move on after failure")
	}
	if _, err := u.SyncAccount(ctx, 2, 3); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("manual sync overlapped worker: %v", err)
	}
	if err := <-results; !errors.Is(err, failure) {
		t.Fatalf("unexpected worker error: %v", err)
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop")
	}
}
