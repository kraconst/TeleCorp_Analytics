package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

const (
	genCompanyID      int64 = 1
	genOtherCompanyID int64 = 2
	genChatID         int64 = 42
	genForeignChatID  int64 = 43
	genMissingChatID  int64 = 999
	genAccountID      int64 = 7
	genForeignAccount int64 = 8
	genSecretText           = "секретный текст переписки"
)

// --- mocks -------------------------------------------------------------------

type genChatRepo struct {
	mu           sync.Mutex
	chats        map[int64]domain.Chat
	anyAccountID int64 // > 0: любой неизвестный chatID принадлежит этому аккаунту
	err          error
	getCalls     int
}

func (r *genChatRepo) Create(context.Context, domain.Chat) (int64, error) {
	return 0, errors.New("genChatRepo.Create: not implemented")
}

func (r *genChatRepo) GetByID(_ context.Context, id int64) (*domain.Chat, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getCalls++
	if r.err != nil {
		return nil, r.err
	}
	if c, ok := r.chats[id]; ok {
		return &c, nil
	}
	if r.anyAccountID > 0 {
		return &domain.Chat{ID: id, AccountID: r.anyAccountID}, nil
	}
	return nil, fmt.Errorf("%w: chat", domain.ErrNotFound)
}

func (r *genChatRepo) ListByCompany(context.Context, int64, *domain.ChatStatus, *int64) ([]domain.Chat, error) {
	return nil, errors.New("genChatRepo.ListByCompany: not implemented")
}

func (r *genChatRepo) ListByFilter(context.Context, domain.ChatFilter) ([]domain.Chat, error) {
	return nil, errors.New("genChatRepo.ListByFilter: not implemented")
}

func (r *genChatRepo) UpdateStatus(context.Context, int64, domain.ChatStatus, domain.StatusSource) error {
	return errors.New("genChatRepo.UpdateStatus: not implemented")
}

func (r *genChatRepo) UpdateAutoStatus(context.Context, int64, domain.ChatStatus) error {
	return errors.New("genChatRepo.UpdateAutoStatus: not implemented")
}

func (r *genChatRepo) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getCalls
}

// genAccountRepo только читается после настройки.
type genAccountRepo struct {
	usecase.TelegramAccountStateRepository
	accounts map[int64]domain.Account
}

func (r *genAccountRepo) Create(context.Context, domain.Account) (int64, error) {
	return 0, errors.New("genAccountRepo.Create: not implemented")
}

func (r *genAccountRepo) GetByID(_ context.Context, id int64) (*domain.Account, error) {
	if a, ok := r.accounts[id]; ok {
		return &a, nil
	}
	return nil, fmt.Errorf("%w: account", domain.ErrNotFound)
}

func (r *genAccountRepo) ListByCompany(context.Context, int64) ([]domain.Account, error) {
	return nil, errors.New("genAccountRepo.ListByCompany: not implemented")
}

func (r *genAccountRepo) ListByEmployee(context.Context, int64) ([]domain.Account, error) {
	return nil, errors.New("genAccountRepo.ListByEmployee: not implemented")
}

func (r *genAccountRepo) UpdateStatus(context.Context, int64, domain.AccountStatus) error {
	return errors.New("genAccountRepo.UpdateStatus: not implemented")
}

func (r *genAccountRepo) AssignEmployee(context.Context, int64, int64) error {
	return errors.New("genAccountRepo.AssignEmployee: not implemented")
}

type genSummaryRepo struct {
	mu              sync.Mutex
	stored          map[int64]domain.ChatSummary
	getErr          error
	upsertErr       error
	upserts         []domain.ChatSummary
	getCalls        int
	storageLocation *time.Location
}

func (r *genSummaryRepo) Upsert(_ context.Context, s domain.ChatSummary) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserts = append(r.upserts, s)
	if r.upsertErr != nil {
		return r.upsertErr
	}
	if r.storageLocation != nil {
		// Имитируем точность timestamptz и часовой пояс при чтении через pgx.
		s.GeneratedAt = s.GeneratedAt.Truncate(time.Microsecond).In(r.storageLocation)
	}
	r.stored[s.ChatID] = s
	return nil
}

func (r *genSummaryRepo) GetByChatID(_ context.Context, _ int64, chatID int64) (*domain.ChatSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getCalls++
	if r.getErr != nil {
		return nil, r.getErr
	}
	if s, ok := r.stored[chatID]; ok {
		return &s, nil
	}
	return nil, fmt.Errorf("%w: chat summary", domain.ErrNotFound)
}

func (r *genSummaryRepo) snapshot() (upserts []domain.ChatSummary, getCalls int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.ChatSummary(nil), r.upserts...), r.getCalls
}

type genMessageRepo struct {
	mu            sync.Mutex
	byChat        map[int64][]domain.Message
	defaultMsgs   []domain.Message // для чатов, отсутствующих в byChat
	err           error
	chatIDs       []int64
	lastCompanyID int64
	lastLimit     int
}

func (r *genMessageRepo) ListLatestByChat(_ context.Context, companyID, chatID int64, limit int) ([]domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chatIDs = append(r.chatIDs, chatID)
	r.lastCompanyID = companyID
	r.lastLimit = limit
	if r.err != nil {
		return nil, r.err
	}
	if msgs, ok := r.byChat[chatID]; ok {
		return append([]domain.Message(nil), msgs...), nil
	}
	return append([]domain.Message(nil), r.defaultMsgs...), nil
}

func (r *genMessageRepo) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.chatIDs)
}

func (r *genMessageRepo) processed() map[int64]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[int64]int, len(r.chatIDs))
	for _, id := range r.chatIDs {
		out[id]++
	}
	return out
}

type genSummarizer struct {
	mu     sync.Mutex
	fn     func(call int, in usecase.AnalyzeChatInput) (*usecase.AIAnalysisResult, error)
	inputs []usecase.AnalyzeChatInput
}

func (s *genSummarizer) AnalyzeChat(_ context.Context, in usecase.AnalyzeChatInput) (*usecase.AIAnalysisResult, error) {
	s.mu.Lock()
	s.inputs = append(s.inputs, in)
	n := len(s.inputs)
	fn := s.fn
	s.mu.Unlock()
	if fn == nil {
		res := genResult()
		return &res, nil
	}
	return fn(n, in)
}

func (s *genSummarizer) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.inputs)
}

func (s *genSummarizer) input(i int) usecase.AnalyzeChatInput {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inputs[i]
}

type genAnalysisCall struct {
	companyID int64
	chatID    int64
	res       usecase.AIAnalysisResult
}

type genAnalysisHandler struct {
	mu    sync.Mutex
	err   error
	calls []genAnalysisCall
}

func (h *genAnalysisHandler) HandleAnalysis(_ context.Context, companyID, chatID int64, res usecase.AIAnalysisResult) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, genAnalysisCall{companyID: companyID, chatID: chatID, res: res})
	return h.err
}

func (h *genAnalysisHandler) snapshot() []genAnalysisCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]genAnalysisCall(nil), h.calls...)
}

// genRetryAfterErr — retryable-ошибка AI с подсказкой Retry-After (как будет в internal/ai).
type genRetryAfterErr struct{ d time.Duration }

func (e genRetryAfterErr) Error() string             { return "ai: rate limited" }
func (e genRetryAfterErr) RetryAfter() time.Duration { return e.d }
func (e genRetryAfterErr) Unwrap() []error {
	return []error{domain.ErrUnavailable, usecase.ErrAIRetryable}
}

var _ usecase.RetryAfterError = genRetryAfterErr{}

// --- helpers -----------------------------------------------------------------

type genEnv struct {
	summaries  *genSummaryRepo
	chats      *genChatRepo
	accounts   *genAccountRepo
	messages   *genMessageRepo
	summarizer *genSummarizer
	analysis   *genAnalysisHandler
}

func genMessages(chatID int64, ids ...int64) []domain.Message {
	base := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	out := make([]domain.Message, 0, len(ids))
	for i, id := range ids {
		dir := domain.MessageDirectionIn
		if i%2 == 1 {
			dir = domain.MessageDirectionOut
		}
		out = append(out, domain.Message{
			ID:                id,
			ChatID:            chatID,
			TelegramMessageID: 1000 + id,
			Direction:         dir,
			Text:              genSecretText,
			SentAt:            base.Add(time.Duration(i) * time.Minute),
		})
	}
	return out
}

func genResult() usecase.AIAnalysisResult {
	status := domain.ChatStatusNegotiation
	return usecase.AIAnalysisResult{
		Topic:           "Поставка оборудования",
		Agreements:      "Цена согласована",
		NextStep:        "Отправить договор",
		OpenQuestions:   "Сроки доставки",
		Risk:            domain.RiskMedium,
		SuggestedStatus: &status,
		AlertTypes:      []domain.AlertType{domain.AlertTypeComplaint},
	}
}

func newGenEnv() *genEnv {
	return &genEnv{
		summaries: &genSummaryRepo{stored: map[int64]domain.ChatSummary{}},
		chats: &genChatRepo{chats: map[int64]domain.Chat{
			genChatID:        {ID: genChatID, AccountID: genAccountID},
			genForeignChatID: {ID: genForeignChatID, AccountID: genForeignAccount},
		}},
		accounts: &genAccountRepo{accounts: map[int64]domain.Account{
			genAccountID:      {ID: genAccountID, CompanyID: genCompanyID},
			genForeignAccount: {ID: genForeignAccount, CompanyID: genOtherCompanyID},
		}},
		messages: &genMessageRepo{byChat: map[int64][]domain.Message{
			genChatID:        genMessages(genChatID, 10, 11, 12),
			genForeignChatID: genMessages(genForeignChatID, 20),
		}},
		summarizer: &genSummarizer{},
	}
}

// uc собирает usecase с AI; analysis передаётся только если задан в env.
func (e *genEnv) uc(maxMessages int) *usecase.ChatSummaryUsecase {
	var analysis usecase.ChatAnalysisHandler
	if e.analysis != nil {
		analysis = e.analysis
	}
	return usecase.NewChatSummaryUsecaseWithAI(e.summaries, e.chats, e.accounts, e.messages, e.summarizer, analysis, maxMessages)
}

func genStoreSummary(e *genEnv, lastMessageID int64) domain.ChatSummary {
	s := domain.ChatSummary{
		ChatID:        genChatID,
		Topic:         "cached topic",
		Agreements:    "cached agreements",
		NextStep:      "cached next step",
		OpenQuestions: "cached questions",
		Risk:          domain.RiskLow,
		LastMessageID: lastMessageID,
		GeneratedAt:   time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
	}
	e.summaries.stored[genChatID] = s
	return s
}

func genStartRun(ctx context.Context, uc *usecase.ChatSummaryUsecase) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		uc.Run(ctx)
		close(done)
	}()
	return done
}

func genAssertDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	default:
		t.Fatal("Run did not return after context cancellation")
	}
}

var genAllSentinels = []error{
	domain.ErrNotFound, domain.ErrForbidden, domain.ErrConflict,
	domain.ErrValidation, domain.ErrUnavailable, domain.ErrBadGateway,
}

// --- Generate ----------------------------------------------------------------

func TestChatSummaryGenerate_InvalidIDs(t *testing.T) {
	cases := []struct {
		name      string
		companyID int64
		chatID    int64
	}{
		{"zero company", 0, genChatID},
		{"negative company", -1, genChatID},
		{"zero chat", genCompanyID, 0},
		{"negative chat", genCompanyID, -5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newGenEnv()
			_, err := env.uc(0).Generate(context.Background(), tc.companyID, tc.chatID, false)
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("expected ErrValidation, got %v", err)
			}
			if env.chats.calls() != 0 || env.summarizer.calls() != 0 {
				t.Fatalf("repositories/summarizer must not be called: chats=%d summarizer=%d", env.chats.calls(), env.summarizer.calls())
			}
		})
	}
}

func TestChatSummaryGenerate_CacheHitSkipsSummarizer(t *testing.T) {
	env := newGenEnv()
	stored := genStoreSummary(env, 12) // совпадает с последним сообщением выборки

	got, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || *got != stored {
		t.Fatalf("expected cached summary %+v, got %+v", stored, got)
	}
	if env.summarizer.calls() != 0 {
		t.Fatalf("summarizer must not be called on cache hit, calls=%d", env.summarizer.calls())
	}
	if upserts, _ := env.summaries.snapshot(); len(upserts) != 0 {
		t.Fatalf("Upsert must not be called on cache hit, got %d", len(upserts))
	}
}

func TestChatSummaryGenerate_CacheComparesLastMessageOfSelection(t *testing.T) {
	env := newGenEnv()
	genStoreSummary(env, 10) // первое, а не последнее сообщение выборки

	if _, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.summarizer.calls() != 1 {
		t.Fatalf("expected 1 summarizer call, got %d", env.summarizer.calls())
	}
}

func TestChatSummaryGenerate_ForceIgnoresCache(t *testing.T) {
	env := newGenEnv()
	stored := genStoreSummary(env, 12)

	got, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.summarizer.calls() != 1 {
		t.Fatalf("expected 1 summarizer call with force, got %d", env.summarizer.calls())
	}
	if prev := env.summarizer.input(0).Previous; prev == nil || *prev != stored {
		t.Fatalf("expected Previous=%+v, got %+v", stored, prev)
	}
	upserts, _ := env.summaries.snapshot()
	if len(upserts) != 1 {
		t.Fatalf("expected 1 Upsert, got %d", len(upserts))
	}
	if got.Topic != genResult().Topic || upserts[0].Topic != genResult().Topic {
		t.Fatalf("new result must be saved and returned, got %+v / %+v", got, upserts[0])
	}
}

func TestChatSummaryGenerate_NewMessagesSavesAndReturnsSummary(t *testing.T) {
	env := newGenEnv()
	genStoreSummary(env, 5)

	before := time.Now().UTC().Truncate(time.Microsecond)
	got, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, false)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.summarizer.calls() != 1 {
		t.Fatalf("expected exactly 1 summarizer call, got %d", env.summarizer.calls())
	}
	in := env.summarizer.input(0)
	if !reflect.DeepEqual(in.Messages, genMessages(genChatID, 10, 11, 12)) {
		t.Fatalf("summarizer got unexpected messages: %+v", in.Messages)
	}
	if in.Previous == nil || in.Previous.LastMessageID != 5 {
		t.Fatalf("expected previous summary with LastMessageID=5, got %+v", in.Previous)
	}

	upserts, _ := env.summaries.snapshot()
	if len(upserts) != 1 {
		t.Fatalf("expected 1 Upsert, got %d", len(upserts))
	}
	saved := upserts[0]
	res := genResult()
	want := domain.ChatSummary{
		ChatID:        genChatID,
		Topic:         res.Topic,
		Agreements:    res.Agreements,
		NextStep:      res.NextStep,
		OpenQuestions: res.OpenQuestions,
		Risk:          res.Risk,
		LastMessageID: 12,
		GeneratedAt:   saved.GeneratedAt,
	}
	if saved != want {
		t.Fatalf("saved summary mismatch:\n got %+v\nwant %+v", saved, want)
	}
	if saved.GeneratedAt.Location() != time.UTC {
		t.Fatalf("GeneratedAt must be UTC, got %v", saved.GeneratedAt.Location())
	}
	if saved.GeneratedAt.Before(before) || saved.GeneratedAt.After(after) {
		t.Fatalf("GeneratedAt %v outside [%v, %v]", saved.GeneratedAt, before, after)
	}
	if got == nil || *got != saved {
		t.Fatalf("response must equal saved summary, got %+v", got)
	}

	env.messages.mu.Lock()
	limit, companyID := env.messages.lastLimit, env.messages.lastCompanyID
	env.messages.mu.Unlock()
	if limit != 50 || companyID != genCompanyID {
		t.Fatalf("ListLatestByChat must use default limit 50 and companyID, got limit=%d company=%d", limit, companyID)
	}
}

func TestChatSummaryGenerate_GeneratedAtMatchesStorage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := newGenEnv()
		storageLocation := time.FixedZone("storage", 3*60*60)
		env.summaries.storageLocation = storageLocation
		uc := env.uc(0)
		ctx := context.Background()

		want := time.Now().UTC().Add(time.Microsecond)
		time.Sleep(time.Microsecond + 234*time.Nanosecond)
		if got := time.Now().UTC().Sub(want); got != 234*time.Nanosecond {
			t.Fatalf("test clock remainder = %v, want 234ns", got)
		}

		generated, err := uc.Generate(ctx, genCompanyID, genChatID, false)
		if err != nil || generated == nil {
			t.Fatalf("Generate: summary=%+v, err=%v", generated, err)
		}
		cached, err := uc.Generate(ctx, genCompanyID, genChatID, false)
		if err != nil || cached == nil {
			t.Fatalf("cached Generate: summary=%+v, err=%v", cached, err)
		}
		fetched, err := uc.GetByChatID(ctx, genCompanyID, genChatID)
		if err != nil || fetched == nil {
			t.Fatalf("GetByChatID: summary=%+v, err=%v", fetched, err)
		}

		upserts, reads := env.summaries.snapshot()
		if len(upserts) != 1 || env.summarizer.calls() != 1 || reads != 3 {
			t.Fatalf("writes=%d, summarizer calls=%d, reads=%d, want 1, 1, 3",
				len(upserts), env.summarizer.calls(), reads)
		}
		if got := env.summaries.stored[genChatID].GeneratedAt; got != want.In(storageLocation) {
			t.Errorf("stored generated_at = %v, want %v", got, want.In(storageLocation))
		}
		for _, check := range []struct {
			source string
			at     time.Time
		}{
			{"Upsert argument", upserts[0].GeneratedAt},
			{"Generate", generated.GeneratedAt},
			{"cached Generate", cached.GeneratedAt},
			{"GetByChatID", fetched.GeneratedAt},
		} {
			if check.at != want {
				t.Errorf("%s generated_at = %v, want %v", check.source, check.at, want)
			}
		}
	})
}

func TestChatSummaryGenerate_NoPreviousSummary(t *testing.T) {
	env := newGenEnv()

	got, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.summarizer.input(0).Previous != nil {
		t.Fatalf("expected nil Previous, got %+v", env.summarizer.input(0).Previous)
	}
	if got.LastMessageID != 12 {
		t.Fatalf("expected LastMessageID=12, got %d", got.LastMessageID)
	}
}

func TestChatSummaryGenerate_MaxMessagesLimit(t *testing.T) {
	cases := []struct {
		in, want int
	}{{20, 20}, {0, 50}, {-3, 50}}
	for _, tc := range cases {
		env := newGenEnv()
		if _, err := env.uc(tc.in).Generate(context.Background(), genCompanyID, genChatID, false); err != nil {
			t.Fatalf("maxMessages=%d: unexpected error: %v", tc.in, err)
		}
		if env.messages.lastLimit != tc.want {
			t.Fatalf("maxMessages=%d: expected limit %d, got %d", tc.in, tc.want, env.messages.lastLimit)
		}
	}
}

func TestChatSummaryGenerate_MissingOrForeignChatNotFound(t *testing.T) {
	for _, chatID := range []int64{genMissingChatID, genForeignChatID} {
		t.Run(fmt.Sprintf("chat_%d", chatID), func(t *testing.T) {
			env := newGenEnv()
			_, err := env.uc(0).Generate(context.Background(), genCompanyID, chatID, true)
			if !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("expected ErrNotFound, got %v", err)
			}
			upserts, getCalls := env.summaries.snapshot()
			if env.summarizer.calls() != 0 || env.messages.calls() != 0 || getCalls != 0 || len(upserts) != 0 {
				t.Fatalf("nothing past company check must run: summarizer=%d messages=%d get=%d upserts=%d",
					env.summarizer.calls(), env.messages.calls(), getCalls, len(upserts))
			}
		})
	}
}

func TestChatSummaryGenerate_RepositoryErrorsWrapped(t *testing.T) {
	dbErr := errors.New("db is down")
	cases := []struct {
		name  string
		setup func(e *genEnv)
	}{
		{"chat repo", func(e *genEnv) { e.chats.err = dbErr }},
		{"message repo", func(e *genEnv) { e.messages.err = dbErr }},
		{"summary get", func(e *genEnv) { e.summaries.getErr = dbErr }},
		{"summary upsert", func(e *genEnv) { e.summaries.upsertErr = dbErr }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newGenEnv()
			env.analysis = &genAnalysisHandler{}
			tc.setup(env)
			got, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, false)
			if !errors.Is(err, dbErr) || got != nil {
				t.Fatalf("expected wrapped db error and nil summary, got %v / %+v", err, got)
			}
			if errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("db error must not be reported as ErrNotFound: %v", err)
			}
			if len(env.analysis.snapshot()) != 0 {
				t.Fatal("HandleAnalysis must not be called on repository error")
			}
		})
	}
}

func TestChatSummaryGenerate_EmptyChatValidation(t *testing.T) {
	env := newGenEnv()
	env.messages.byChat[genChatID] = []domain.Message{}

	_, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, true)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
	if env.summarizer.calls() != 0 {
		t.Fatalf("summarizer must not be called for empty chat, calls=%d", env.summarizer.calls())
	}
	if _, getCalls := env.summaries.snapshot(); getCalls != 0 {
		t.Fatalf("summary cache must not be read before messages check, calls=%d", getCalls)
	}
}

func TestChatSummaryGenerate_AIDisabledUnavailable(t *testing.T) {
	t.Run("nil summarizer with up-to-date cache", func(t *testing.T) {
		env := newGenEnv()
		genStoreSummary(env, 12)
		uc := usecase.NewChatSummaryUsecaseWithAI(env.summaries, env.chats, env.accounts, env.messages, nil, nil, 0)
		_, err := uc.Generate(context.Background(), genCompanyID, genChatID, false)
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("expected ErrUnavailable, got %v", err)
		}
		if _, getCalls := env.summaries.snapshot(); env.messages.calls() != 0 || getCalls != 0 {
			t.Fatalf("messages/cache must not be read when AI disabled: messages=%d get=%d", env.messages.calls(), getCalls)
		}
		if env.chats.calls() != 1 {
			t.Fatalf("chat/company check must run before ErrUnavailable, chats calls=%d", env.chats.calls())
		}
	})
	t.Run("typed nil summarizer", func(t *testing.T) {
		env := newGenEnv()
		var typedNil *genSummarizer
		uc := usecase.NewChatSummaryUsecaseWithAI(env.summaries, env.chats, env.accounts, env.messages, typedNil, nil, 0)
		_, err := uc.Generate(context.Background(), genCompanyID, genChatID, false)
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("expected ErrUnavailable, got %v", err)
		}
	})
	t.Run("nil message repository", func(t *testing.T) {
		env := newGenEnv()
		uc := usecase.NewChatSummaryUsecaseWithAI(env.summaries, env.chats, env.accounts, nil, env.summarizer, nil, 0)
		_, err := uc.Generate(context.Background(), genCompanyID, genChatID, false)
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("expected ErrUnavailable, got %v", err)
		}
		if env.summarizer.calls() != 0 {
			t.Fatal("summarizer must not be called without message repository")
		}
	})
	t.Run("legacy constructor", func(t *testing.T) {
		env := newGenEnv()
		uc := usecase.NewChatSummaryUsecase(env.summaries, env.chats, env.accounts)
		_, err := uc.Generate(context.Background(), genCompanyID, genChatID, false)
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("expected ErrUnavailable, got %v", err)
		}
	})
	t.Run("foreign chat checked before AI disabled", func(t *testing.T) {
		env := newGenEnv()
		uc := usecase.NewChatSummaryUsecase(env.summaries, env.chats, env.accounts)
		_, err := uc.Generate(context.Background(), genCompanyID, genForeignChatID, false)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound before ErrUnavailable, got %v", err)
		}
	})
}

func TestChatSummaryGenerate_MissingRepositoriesInternalError(t *testing.T) {
	env := newGenEnv()
	ucs := map[string]*usecase.ChatSummaryUsecase{
		"legacy nil chats/accounts": usecase.NewChatSummaryUsecase(env.summaries, nil, nil),
		"ai nil chats/accounts":     usecase.NewChatSummaryUsecaseWithAI(env.summaries, nil, nil, env.messages, env.summarizer, nil, 0),
		"ai nil accounts":           usecase.NewChatSummaryUsecaseWithAI(env.summaries, env.chats, nil, env.messages, env.summarizer, nil, 0),
	}
	for name, uc := range ucs {
		t.Run(name, func(t *testing.T) {
			_, err := uc.Generate(context.Background(), genCompanyID, genChatID, false)
			if err == nil {
				t.Fatal("expected configuration error")
			}
			for _, s := range genAllSentinels {
				if errors.Is(err, s) {
					t.Fatalf("configuration error must not carry domain sentinel %v: %v", s, err)
				}
			}
		})
	}
	if env.summarizer.calls() != 0 || env.messages.calls() != 0 {
		t.Fatalf("company check must not be skipped: summarizer=%d messages=%d", env.summarizer.calls(), env.messages.calls())
	}
}

func TestChatSummaryGenerate_SummarizerErrorNotSavedAndNotRetried(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"timeout", fmt.Errorf("%w: request timeout", domain.ErrUnavailable), domain.ErrUnavailable},
		{"retryable", fmt.Errorf("%w: %w", domain.ErrUnavailable, usecase.ErrAIRetryable), usecase.ErrAIRetryable},
		{"retry-after", genRetryAfterErr{d: time.Second}, domain.ErrUnavailable},
		{"bad gateway", fmt.Errorf("%w: invalid json", domain.ErrBadGateway), domain.ErrBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newGenEnv()
			env.analysis = &genAnalysisHandler{}
			env.summarizer.fn = func(int, usecase.AnalyzeChatInput) (*usecase.AIAnalysisResult, error) {
				return nil, tc.err
			}
			got, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, false)
			if !errors.Is(err, tc.want) || got != nil {
				t.Fatalf("expected %v, got %v / %+v", tc.want, err, got)
			}
			if env.summarizer.calls() != 1 {
				t.Fatalf("synchronous Generate must not retry, calls=%d", env.summarizer.calls())
			}
			if upserts, _ := env.summaries.snapshot(); len(upserts) != 0 {
				t.Fatalf("Upsert must not be called on summarizer error, got %d", len(upserts))
			}
			if len(env.analysis.snapshot()) != 0 {
				t.Fatal("HandleAnalysis must not be called on summarizer error")
			}
		})
	}
}

func TestChatSummaryGenerate_NilResultBadGateway(t *testing.T) {
	env := newGenEnv()
	env.summarizer.fn = func(int, usecase.AnalyzeChatInput) (*usecase.AIAnalysisResult, error) {
		return nil, nil
	}
	_, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, false)
	if !errors.Is(err, domain.ErrBadGateway) {
		t.Fatalf("expected ErrBadGateway, got %v", err)
	}
	if upserts, _ := env.summaries.snapshot(); len(upserts) != 0 {
		t.Fatalf("Upsert must not be called, got %d", len(upserts))
	}
}

func TestChatSummaryGenerate_InvalidRiskBadGateway(t *testing.T) {
	for _, risk := range []domain.RiskLevel{"", "High", " high", "critical"} {
		t.Run(fmt.Sprintf("risk_%q", risk), func(t *testing.T) {
			env := newGenEnv()
			env.analysis = &genAnalysisHandler{}
			env.summarizer.fn = func(int, usecase.AnalyzeChatInput) (*usecase.AIAnalysisResult, error) {
				res := genResult()
				res.Risk = risk
				return &res, nil
			}
			_, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, false)
			if !errors.Is(err, domain.ErrBadGateway) {
				t.Fatalf("expected ErrBadGateway, got %v", err)
			}
			if upserts, _ := env.summaries.snapshot(); len(upserts) != 0 {
				t.Fatalf("Upsert must not be called for invalid risk, got %d", len(upserts))
			}
			if len(env.analysis.snapshot()) != 0 {
				t.Fatal("HandleAnalysis must not be called for invalid risk")
			}
		})
	}
}

func TestChatSummaryGenerate_AnalysisHandlerCalled(t *testing.T) {
	env := newGenEnv()
	env.analysis = &genAnalysisHandler{}

	if _, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	calls := env.analysis.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected 1 HandleAnalysis call, got %d", len(calls))
	}
	c := calls[0]
	if c.companyID != genCompanyID || c.chatID != genChatID || !reflect.DeepEqual(c.res, genResult()) {
		t.Fatalf("unexpected HandleAnalysis call: %+v", c)
	}
}

func TestChatSummaryGenerate_AnalysisHandlerErrorLoggedNotReturned(t *testing.T) {
	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})

	env := newGenEnv()
	env.analysis = &genAnalysisHandler{err: errors.New("auto status failed")}

	got, err := env.uc(0).Generate(context.Background(), genCompanyID, genChatID, false)
	if err != nil {
		t.Fatalf("handler error must not be returned, got %v", err)
	}
	upserts, _ := env.summaries.snapshot()
	if len(upserts) != 1 || got == nil || *got != upserts[0] {
		t.Fatalf("summary must stay saved and be returned: upserts=%d got=%+v", len(upserts), got)
	}
	out := buf.String()
	if !strings.Contains(out, "company_id=1") || !strings.Contains(out, "chat_id=42") || !strings.Contains(out, "auto status failed") {
		t.Fatalf("log must contain company_id, chat_id and error, got %q", out)
	}
	if strings.Contains(out, genSecretText) || strings.Contains(out, genResult().Topic) {
		t.Fatalf("log must not contain conversation text or provider output, got %q", out)
	}
}

func TestChatSummaryUpsert_LegacyBehaviourUnchanged(t *testing.T) {
	env := newGenEnv()
	uc := usecase.NewChatSummaryUsecase(env.summaries, env.chats, env.accounts)
	if err := uc.Upsert(context.Background(), genCompanyID, domain.ChatSummary{ChatID: genChatID, Topic: "  t  "}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	upserts, _ := env.summaries.snapshot()
	if len(upserts) != 1 || upserts[0].Risk != domain.RiskLow || upserts[0].Topic != "t" {
		t.Fatalf("legacy Upsert must default risk to low and trim, got %+v", upserts)
	}
	got, err := uc.GetByChatID(context.Background(), genCompanyID, genChatID)
	if err != nil || got.Topic != "t" {
		t.Fatalf("legacy GetByChatID failed: %v / %+v", err, got)
	}
	if _, err := uc.GetByChatID(context.Background(), genCompanyID, genForeignChatID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for foreign chat, got %v", err)
	}
}

// --- Enqueue / Run -----------------------------------------------------------

func TestChatSummaryEnqueue_DedupAndRequeueAfterTake(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := newGenEnv()
		uc := env.uc(0)
		for range 3 {
			uc.Enqueue(context.Background(), genCompanyID, genChatID)
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := genStartRun(ctx, uc)
		synctest.Wait()
		if n := env.messages.calls(); n != 1 {
			t.Fatalf("duplicate Enqueue must produce one job, got %d", n)
		}

		// После взятия задачи воркером чат снова можно поставить в очередь.
		uc.Enqueue(context.Background(), genCompanyID, genChatID)
		synctest.Wait()
		if n := env.messages.calls(); n != 2 {
			t.Fatalf("expected second job after take, got %d", n)
		}

		cancel()
		synctest.Wait()
		genAssertDone(t, done)
	})
}

func TestChatSummaryEnqueue_NonBlockingOverflowDropped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := newGenEnv()
		env.chats.anyAccountID = genAccountID
		env.messages.defaultMsgs = genMessages(0, 1)
		uc := env.uc(0)

		const first, total = int64(1000), 101
		// Воркер не запущен: без неблокирующей отправки synctest обнаружил бы deadlock.
		for i := range int64(total) {
			uc.Enqueue(context.Background(), genCompanyID, first+i)
		}
		overflowID := first + total - 1

		ctx, cancel := context.WithCancel(context.Background())
		done := genStartRun(ctx, uc)
		synctest.Wait()

		processed := env.messages.processed()
		if len(processed) != 100 {
			t.Fatalf("expected 100 processed jobs (buffer size), got %d", len(processed))
		}
		if _, ok := processed[overflowID]; ok {
			t.Fatalf("overflow job %d must be dropped", overflowID)
		}

		// Отброшенный чат не остаётся в множестве «в очереди».
		uc.Enqueue(context.Background(), genCompanyID, overflowID)
		synctest.Wait()
		if n := env.messages.processed()[overflowID]; n != 1 {
			t.Fatalf("dropped chat must be enqueueable again, processed=%d", n)
		}

		cancel()
		synctest.Wait()
		genAssertDone(t, done)
	})
}

func TestChatSummaryEnqueue_InvalidIDsDropped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := newGenEnv()
		uc := env.uc(0)
		uc.Enqueue(context.Background(), 0, genChatID)
		uc.Enqueue(context.Background(), genCompanyID, 0)
		uc.Enqueue(context.Background(), -1, -1)

		ctx, cancel := context.WithCancel(context.Background())
		done := genStartRun(ctx, uc)
		synctest.Wait()
		if n := env.chats.calls(); n != 0 {
			t.Fatalf("invalid IDs must be dropped, chats calls=%d", n)
		}

		uc.Enqueue(context.Background(), genCompanyID, genChatID)
		synctest.Wait()
		if n := env.messages.calls(); n != 1 {
			t.Fatalf("valid job after invalid ones must be processed, got %d", n)
		}

		cancel()
		synctest.Wait()
		genAssertDone(t, done)
	})
}

func TestChatSummaryEnqueue_IgnoresCallerContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := newGenEnv()
		uc := env.uc(0)
		callerCtx, callerCancel := context.WithCancel(context.Background())
		callerCancel()
		uc.Enqueue(callerCtx, genCompanyID, genChatID)

		ctx, cancel := context.WithCancel(context.Background())
		done := genStartRun(ctx, uc)
		synctest.Wait()
		if env.summarizer.calls() != 1 {
			t.Fatalf("job must run on worker context, summarizer calls=%d", env.summarizer.calls())
		}
		if upserts, _ := env.summaries.snapshot(); len(upserts) != 1 || upserts[0].LastMessageID != 12 {
			t.Fatalf("background generation must save summary, got %+v", upserts)
		}
		cancel()
		synctest.Wait()
		genAssertDone(t, done)
	})
}

func TestChatSummaryEnqueue_AIDisabledNoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := newGenEnv()
		uc := usecase.NewChatSummaryUsecase(env.summaries, env.chats, env.accounts)
		for range 200 {
			uc.Enqueue(context.Background(), genCompanyID, genChatID)
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := genStartRun(ctx, uc)
		synctest.Wait()
		if n := env.chats.calls(); n != 0 {
			t.Fatalf("Enqueue must be no-op when AI disabled, chats calls=%d", n)
		}
		cancel()
		synctest.Wait()
		genAssertDone(t, done)
	})
}

func TestChatSummaryRun_StopsOnCancelAndDropsPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := newGenEnv()
		uc := env.uc(0)

		ctx, cancel := context.WithCancel(context.Background())
		done := genStartRun(ctx, uc)
		synctest.Wait()
		cancel()
		synctest.Wait()
		genAssertDone(t, done)

		// Отменённый контекст до старта: задачи в очереди отбрасываются.
		uc.Enqueue(context.Background(), genCompanyID, genChatID)
		uc.Run(ctx)
		if n := env.chats.calls(); n != 0 {
			t.Fatalf("pending jobs must be dropped after cancellation, chats calls=%d", n)
		}
	})
}

func TestChatSummaryRun_RetriesOnceForRetryable(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		delay time.Duration
	}{
		{"retry-after 5s", genRetryAfterErr{d: 5 * time.Second}, 5 * time.Second},
		{"retry-after 10s boundary", genRetryAfterErr{d: 10 * time.Second}, 10 * time.Second},
		{"retry-after above max", genRetryAfterErr{d: 15 * time.Second}, 2 * time.Second},
		{"retry-after zero", genRetryAfterErr{d: 0}, 0},
		{"retry-after negative", genRetryAfterErr{d: -time.Second}, 2 * time.Second},
		{"no retry-after", fmt.Errorf("%w: %w", domain.ErrUnavailable, usecase.ErrAIRetryable), 2 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				env := newGenEnv()
				env.summarizer.fn = func(call int, _ usecase.AnalyzeChatInput) (*usecase.AIAnalysisResult, error) {
					if call == 1 {
						return nil, tc.err
					}
					res := genResult()
					return &res, nil
				}
				uc := env.uc(0)
				uc.Enqueue(context.Background(), genCompanyID, genChatID)

				ctx, cancel := context.WithCancel(context.Background())
				done := genStartRun(ctx, uc)
				defer func() {
					cancel()
					synctest.Wait()
					genAssertDone(t, done)
				}()
				synctest.Wait()
				if tc.delay > 0 {
					if n := env.summarizer.calls(); n != 1 {
						t.Fatalf("expected first attempt only, got %d", n)
					}

					time.Sleep(tc.delay - time.Nanosecond)
					synctest.Wait()
					if n := env.summarizer.calls(); n != 1 {
						t.Fatalf("retry happened before backoff %v, calls=%d", tc.delay, n)
					}

					time.Sleep(time.Nanosecond)
					synctest.Wait()
				}
				if n := env.summarizer.calls(); n != 2 {
					t.Fatalf("expected retry after %v, calls=%d", tc.delay, n)
				}
				if upserts, _ := env.summaries.snapshot(); len(upserts) != 1 {
					t.Fatalf("successful retry must save summary, upserts=%d", len(upserts))
				}

				time.Sleep(time.Minute)
				synctest.Wait()
				if n := env.summarizer.calls(); n != 2 {
					t.Fatalf("no further attempts expected, calls=%d", n)
				}
			})
		})
	}
}

func TestChatSummaryRun_RetriesAtMostOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := newGenEnv()
		env.summarizer.fn = func(int, usecase.AnalyzeChatInput) (*usecase.AIAnalysisResult, error) {
			return nil, fmt.Errorf("%w: %w", domain.ErrUnavailable, usecase.ErrAIRetryable)
		}
		uc := env.uc(0)
		uc.Enqueue(context.Background(), genCompanyID, genChatID)

		ctx, cancel := context.WithCancel(context.Background())
		done := genStartRun(ctx, uc)
		time.Sleep(time.Minute)
		synctest.Wait()
		if n := env.summarizer.calls(); n != 2 {
			t.Fatalf("expected exactly 2 attempts (1 retry), got %d", n)
		}
		if upserts, _ := env.summaries.snapshot(); len(upserts) != 0 {
			t.Fatalf("failed generation must not save, upserts=%d", len(upserts))
		}
		cancel()
		synctest.Wait()
		genAssertDone(t, done)
	})
}

func TestChatSummaryRun_NoRetryForNonRetryable(t *testing.T) {
	summarizerErr := func(err error) func(e *genEnv) {
		return func(e *genEnv) {
			e.summarizer.fn = func(int, usecase.AnalyzeChatInput) (*usecase.AIAnalysisResult, error) {
				return nil, err
			}
		}
	}
	cases := []struct {
		name   string
		chatID int64
		setup  func(e *genEnv)
		count  func(e *genEnv) int
	}{
		{"timeout", genChatID, summarizerErr(fmt.Errorf("%w: timeout", domain.ErrUnavailable)),
			func(e *genEnv) int { return e.summarizer.calls() }},
		{"bad gateway", genChatID, summarizerErr(fmt.Errorf("%w: invalid json", domain.ErrBadGateway)),
			func(e *genEnv) int { return e.summarizer.calls() }},
		{"bad gateway with retryable marker", genChatID,
			summarizerErr(fmt.Errorf("%w: %w", domain.ErrBadGateway, usecase.ErrAIRetryable)),
			func(e *genEnv) int { return e.summarizer.calls() }},
		{"not found", genMissingChatID, func(*genEnv) {},
			func(e *genEnv) int { return e.chats.calls() }},
		{"validation (empty chat)", genChatID, func(e *genEnv) { e.messages.byChat[genChatID] = nil },
			func(e *genEnv) int { return e.messages.calls() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				env := newGenEnv()
				tc.setup(env)
				uc := env.uc(0)
				uc.Enqueue(context.Background(), genCompanyID, tc.chatID)

				ctx, cancel := context.WithCancel(context.Background())
				done := genStartRun(ctx, uc)
				time.Sleep(time.Minute)
				synctest.Wait()
				if n := tc.count(env); n != 1 {
					t.Fatalf("expected exactly 1 attempt without retry, got %d", n)
				}
				cancel()
				synctest.Wait()
				genAssertDone(t, done)
			})
		})
	}
}

func TestChatSummaryRun_CancelDuringBackoffNoRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := newGenEnv()
		env.summarizer.fn = func(int, usecase.AnalyzeChatInput) (*usecase.AIAnalysisResult, error) {
			return nil, genRetryAfterErr{d: 5 * time.Second}
		}
		uc := env.uc(0)
		uc.Enqueue(context.Background(), genCompanyID, genChatID)

		ctx, cancel := context.WithCancel(context.Background())
		done := genStartRun(ctx, uc)
		synctest.Wait()
		if n := env.summarizer.calls(); n != 1 {
			t.Fatalf("expected first attempt, got %d", n)
		}

		time.Sleep(time.Second)
		cancel()
		synctest.Wait()
		genAssertDone(t, done)

		time.Sleep(time.Minute)
		if n := env.summarizer.calls(); n != 1 {
			t.Fatalf("cancellation during backoff must prevent retry, calls=%d", n)
		}
	})
}
