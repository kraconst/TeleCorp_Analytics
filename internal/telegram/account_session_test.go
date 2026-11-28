package telegram

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/session"
	"github.com/kraconst/GO_server/internal/config"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type sessionRepositoryStub struct {
	usecase.TelegramAccountStateRepository
	value     []byte
	loadErr   error
	statusErr error
	scope     [2]int64
	status    domain.AccountStatus
}

func (s *sessionRepositoryStub) LoadSession(_ context.Context, companyID, accountID int64) ([]byte, error) {
	s.scope = [2]int64{companyID, accountID}
	return s.value, s.loadErr
}

func (s *sessionRepositoryStub) UpdateSession(_ context.Context, companyID, accountID int64, value []byte) error {
	s.scope = [2]int64{companyID, accountID}
	s.value = bytes.Clone(value)
	return nil
}

func (s *sessionRepositoryStub) UpdateSessionStatus(_ context.Context, companyID, accountID int64, status domain.AccountStatus) error {
	s.scope = [2]int64{companyID, accountID}
	s.status = status
	return s.statusErr
}

func TestAccountSessionEncryptionAndScope(t *testing.T) {
	ctx := t.Context()
	repo := &sessionRepositoryStub{}
	key := bytes.Repeat([]byte{1}, 32)
	storage := func(companyID, accountID int64, key []byte) *AccountSessionStorage {
		t.Helper()
		storage, err := NewAccountSessionStorage(repo, companyID, accountID, key)
		if err != nil {
			t.Fatal(err)
		}
		return storage
	}
	primary := storage(2, 6, key)
	data := []byte("demo session contents")
	if _, err := primary.LoadSession(ctx); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("missing session: %v", err)
	}
	if err := primary.StoreSession(ctx, data); err != nil {
		t.Fatal(err)
	}
	first := bytes.Clone(repo.value)
	if repo.scope != [2]int64{2, 6} || bytes.Contains(first, data) {
		t.Fatal("session was not scoped and encrypted")
	}
	if err := primary.StoreSession(ctx, data); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, repo.value) {
		t.Fatal("identical sessions must be written with different nonces")
	}
	loaded, err := primary.LoadSession(ctx)
	if err != nil || !bytes.Equal(loaded, data) {
		t.Fatalf("encrypted session did not round trip: %v", err)
	}
	for _, other := range []*AccountSessionStorage{
		storage(3, 6, key), storage(2, 7, key), storage(2, 6, bytes.Repeat([]byte{2}, 32)),
	} {
		if _, err := other.LoadSession(ctx); !errors.Is(err, errUnreadableAccountSession) {
			t.Fatalf("different scope or key could read the session: %v", err)
		}
	}
	repo.value[len(repo.value)-1] ^= 1
	if _, err := primary.LoadSession(ctx); !errors.Is(err, errUnreadableAccountSession) {
		t.Fatalf("modified ciphertext was accepted: %v", err)
	}
}

func TestWithAccountPreflightClassifiesOnlyMissingSession(t *testing.T) {
	statusFailure := errors.New("status write failed")
	for _, tc := range []struct {
		name      string
		value     []byte
		loadErr   error
		statusErr error
		wantErr   error
		status    domain.AccountStatus
	}{
		{name: "missing account", loadErr: domain.ErrNotFound, wantErr: domain.ErrNotFound},
		{name: "missing session", wantErr: usecase.ErrTelegramSessionInvalid, status: domain.StatusDisconnected},
		{name: "failed status write", statusErr: statusFailure, wantErr: statusFailure, status: domain.StatusDisconnected},
		{name: "unreadable ciphertext", value: []byte{1, 2, 3}, wantErr: errUnreadableAccountSession, status: domain.StatusDisconnected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &sessionRepositoryStub{value: tc.value, loadErr: tc.loadErr, statusErr: tc.statusErr}
			client, err := NewSyncClient(config.TelegramConfig{
				APIID: 1, APIHash: "test", SyncBatchLimit: 100, SessionKey: make([]byte, 32),
			}, repo)
			if err != nil {
				t.Fatal(err)
			}
			err = client.WithAccount(t.Context(), 2, 6, func(context.Context, usecase.TelegramAccountReader) error {
				t.Fatal("unsuccessful preflight started a connection")
				return nil
			})
			if !errors.Is(err, tc.wantErr) || repo.status != tc.status || repo.scope != [2]int64{2, 6} {
				t.Fatalf("failure = %v, status = %q, scope = %v", err, repo.status, repo.scope)
			}
			if tc.statusErr != nil && !errors.Is(err, usecase.ErrTelegramSessionInvalid) {
				t.Fatalf("status write failure did not preserve session failure: %v", err)
			}
		})
	}
}

func TestVerifySavedAuthKeyRejectsMalformedAndReplacedKeys(t *testing.T) {
	ctx := t.Context()
	loader := &session.Loader{Storage: &session.StorageMemory{}}
	var first, second crypto.Key
	first[0], second[0] = 1, 2
	firstID, secondID := first.ID(), second.ID()
	if err := loader.Save(ctx, &session.Data{AuthKey: first[:], AuthKeyID: firstID[:]}); err != nil {
		t.Fatal(err)
	}
	if err := verifySavedAuthKey(ctx, loader, first); err != nil {
		t.Fatal(err)
	}
	if err := loader.Save(ctx, &session.Data{AuthKey: second[:], AuthKeyID: secondID[:]}); err != nil {
		t.Fatal(err)
	}
	if err := verifySavedAuthKey(ctx, loader, first); !errors.Is(err, usecase.ErrTelegramSessionInvalid) {
		t.Fatalf("a replaced key was accepted: %v", err)
	}
	if err := loader.Save(ctx, &session.Data{AuthKey: first[:], AuthKeyID: secondID[:]}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSavedAuthKey(ctx, loader); !errors.Is(err, errUnreadableAccountSession) {
		t.Fatalf("a corrupted key was accepted: %v", err)
	}
}
