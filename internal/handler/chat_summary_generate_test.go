package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/usecase"
)

// secretMarker — строка в ошибке summarizer, которая не должна попасть в тело ответа (RFC 02, 2A-8a).
const secretMarker = "PROVIDER-SECRET-MARKER-sk-test-123"

// --- mocks ---

type genChatRepo struct {
	usecase.ChatRepository
	chats map[int64]domain.Chat
}

func (m *genChatRepo) GetByID(_ context.Context, id int64) (*domain.Chat, error) {
	ch, ok := m.chats[id]
	if !ok {
		return nil, fmt.Errorf("%w: chat %d", domain.ErrNotFound, id)
	}
	return &ch, nil
}

type genAccountRepo struct {
	usecase.AccountRepository
	accounts map[int64]domain.Account
}

func (m *genAccountRepo) GetByID(_ context.Context, id int64) (*domain.Account, error) {
	acc, ok := m.accounts[id]
	if !ok {
		return nil, fmt.Errorf("%w: account %d", domain.ErrNotFound, id)
	}
	return &acc, nil
}

type genSummaryRepo struct {
	mu          sync.Mutex
	stored      map[int64]domain.ChatSummary
	upsertCalls int
}

func (m *genSummaryRepo) Upsert(_ context.Context, s domain.ChatSummary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upsertCalls++
	m.stored[s.ChatID] = s
	return nil
}

func (m *genSummaryRepo) GetByChatID(_ context.Context, _, chatID int64) (*domain.ChatSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.stored[chatID]
	if !ok {
		return nil, fmt.Errorf("%w: summary for chat %d", domain.ErrNotFound, chatID)
	}
	return &s, nil
}

func (m *genSummaryRepo) upserts() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.upsertCalls
}

type genMessageRepo struct {
	byChat map[int64][]domain.Message
}

func (m *genMessageRepo) ListLatestByChat(_ context.Context, _, chatID int64, limit int) ([]domain.Message, error) {
	msgs := m.byChat[chatID]
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	out := make([]domain.Message, len(msgs))
	copy(out, msgs)
	return out, nil
}

type genSummarizer struct {
	mu    sync.Mutex
	calls int
	err   error
	res   *usecase.AIAnalysisResult
}

func (m *genSummarizer) AnalyzeChat(_ context.Context, _ usecase.AnalyzeChatInput) (*usecase.AIAnalysisResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.res, nil
}

func (m *genSummarizer) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// --- fixture ---

const (
	genCompanyID      = int64(1)
	genOtherCompanyID = int64(2)
	genChatWithMsgs   = int64(10) // чат компании 1, последнее сообщение 102
	genChatEmpty      = int64(11) // чат компании 1 без сообщений
	genChatForeign    = int64(12) // чат компании 2
	genLastMessageID  = int64(102)
)

type genFixture struct {
	summaries  *genSummaryRepo
	summarizer *genSummarizer
	chats      *genChatRepo
	accounts   *genAccountRepo
	messages   *genMessageRepo
}

func newGenFixture() *genFixture {
	return &genFixture{
		summaries: &genSummaryRepo{stored: map[int64]domain.ChatSummary{}},
		summarizer: &genSummarizer{res: &usecase.AIAnalysisResult{
			Topic:         "new topic",
			Agreements:    "new agreements",
			NextStep:      "new next step",
			OpenQuestions: "",
			Risk:          domain.RiskMedium,
		}},
		chats: &genChatRepo{chats: map[int64]domain.Chat{
			genChatWithMsgs: {ID: genChatWithMsgs, AccountID: 100},
			genChatEmpty:    {ID: genChatEmpty, AccountID: 100},
			genChatForeign:  {ID: genChatForeign, AccountID: 200},
		}},
		accounts: &genAccountRepo{accounts: map[int64]domain.Account{
			100: {ID: 100, CompanyID: genCompanyID},
			200: {ID: 200, CompanyID: genOtherCompanyID},
		}},
		messages: &genMessageRepo{byChat: map[int64][]domain.Message{
			genChatWithMsgs: {
				{ID: 101, ChatID: genChatWithMsgs, Direction: domain.MessageDirectionIn, Text: "hello", SentAt: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)},
				{ID: genLastMessageID, ChatID: genChatWithMsgs, Direction: domain.MessageDirectionOut, Text: "hi", SentAt: time.Date(2026, 11, 1, 10, 1, 0, 0, time.UTC)},
			},
		}},
	}
}

// withCachedSummary кладёт сохранённое саммари с указанным last_message_id.
func (f *genFixture) withCachedSummary(lastMessageID int64) domain.ChatSummary {
	s := domain.ChatSummary{
		ChatID:        genChatWithMsgs,
		Topic:         "cached topic",
		Risk:          domain.RiskLow,
		LastMessageID: lastMessageID,
		GeneratedAt:   time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
	}
	f.summaries.stored[genChatWithMsgs] = s
	return s
}

func (f *genFixture) aiRouter() http.Handler {
	uc := usecase.NewChatSummaryUsecaseWithAI(f.summaries, f.chats, f.accounts, f.messages, f.summarizer, nil, 50)
	return handler.New(nil, nil, nil, nil, nil, nil, nil, nil, uc, nil).InitRoutes()
}

func (f *genFixture) noAIRouter() http.Handler {
	uc := usecase.NewChatSummaryUsecase(f.summaries, f.chats, f.accounts)
	return handler.New(nil, nil, nil, nil, nil, nil, nil, nil, uc, nil).InitRoutes()
}

func doGenRequest(r http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func generatePath(chatID string, query string) string {
	p := "/chats/" + chatID + "/summary/generate"
	if query != "" {
		p += "?" + query
	}
	return p
}

func decodeSummary(t *testing.T, w *httptest.ResponseRecorder) domain.ChatSummary {
	t.Helper()
	var s domain.ChatSummary
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode summary: %v, body: %s", err, w.Body.String())
	}
	return s
}

func assertErrorBody(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v, body: %s", err, w.Body.String())
	}
	msg, ok := body["error"].(string)
	if !ok || msg == "" {
		t.Fatalf("expected non-empty string field \"error\", got body: %s", w.Body.String())
	}
	if len(body) != 1 {
		t.Fatalf("expected only \"error\" field, got body: %s", w.Body.String())
	}
	return msg
}

// --- tests: success and cache ---

// 2A-4: новые сообщения → один вызов LLM, сохранено с last_message_id последнего сообщения, generated_at в UTC.
func TestGenerateChatSummary_NewMessages_CallsLLMOnce(t *testing.T) {
	f := newGenFixture()
	f.withCachedSummary(101) // устаревший кэш
	r := f.aiRouter()

	w := doGenRequest(r, http.MethodPost, generatePath("10", "company_id=1"), []byte(`{"ignored":true}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if got := f.summarizer.callCount(); got != 1 {
		t.Fatalf("expected 1 summarizer call, got %d", got)
	}
	if got := f.summaries.upserts(); got != 1 {
		t.Fatalf("expected 1 upsert, got %d", got)
	}
	s := decodeSummary(t, w)
	if s.ChatID != genChatWithMsgs || s.LastMessageID != genLastMessageID || s.Topic != "new topic" || s.Risk != domain.RiskMedium {
		t.Fatalf("unexpected summary in response: %+v", s)
	}
	stored := f.summaries.stored[genChatWithMsgs]
	if stored.LastMessageID != genLastMessageID || stored.Topic != "new topic" {
		t.Fatalf("unexpected stored summary: %+v", stored)
	}
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	ga, _ := raw["generated_at"].(string)
	if !strings.HasSuffix(ga, "Z") {
		t.Fatalf("expected generated_at in UTC (suffix Z), got %q", ga)
	}
}

// 2A-4: саммари ещё нет (GetByChatID → ErrNotFound) — тоже генерация.
func TestGenerateChatSummary_NoPreviousSummary_Generates(t *testing.T) {
	f := newGenFixture()
	r := f.aiRouter()

	w := doGenRequest(r, http.MethodPost, generatePath("10", ""), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if got := f.summarizer.callCount(); got != 1 {
		t.Fatalf("expected 1 summarizer call, got %d", got)
	}
	if s := decodeSummary(t, w); s.LastMessageID != genLastMessageID {
		t.Fatalf("expected last_message_id %d, got %d", genLastMessageID, s.LastMessageID)
	}
}

// 2A-2: актуальный кэш без force → 200 с сохранённым, 0 вызовов LLM; force=false явно — так же.
func TestGenerateChatSummary_CacheHit_NoLLMCall(t *testing.T) {
	for _, query := range []string{"company_id=1", "company_id=1&force=false"} {
		t.Run(query, func(t *testing.T) {
			f := newGenFixture()
			cached := f.withCachedSummary(genLastMessageID)
			r := f.aiRouter()

			w := doGenRequest(r, http.MethodPost, generatePath("10", query), nil)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
			}
			if got := f.summarizer.callCount(); got != 0 {
				t.Fatalf("expected 0 summarizer calls, got %d", got)
			}
			if got := f.summaries.upserts(); got != 0 {
				t.Fatalf("expected 0 upserts, got %d", got)
			}
			s := decodeSummary(t, w)
			if s.Topic != cached.Topic || s.LastMessageID != cached.LastMessageID {
				t.Fatalf("expected cached summary %+v, got %+v", cached, s)
			}
		})
	}
}

// 2A-3: force=true при совпадении last_message_id → 1 вызов LLM и сохранение нового результата.
func TestGenerateChatSummary_ForceTrue_BypassesCache(t *testing.T) {
	f := newGenFixture()
	f.withCachedSummary(genLastMessageID)
	r := f.aiRouter()

	w := doGenRequest(r, http.MethodPost, generatePath("10", "force=true&company_id=1"), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if got := f.summarizer.callCount(); got != 1 {
		t.Fatalf("expected 1 summarizer call, got %d", got)
	}
	if got := f.summaries.upserts(); got != 1 {
		t.Fatalf("expected 1 upsert, got %d", got)
	}
	if s := decodeSummary(t, w); s.Topic != "new topic" {
		t.Fatalf("expected regenerated summary, got %+v", s)
	}
}

// --- tests: 4xx ---

// 2A-5: несуществующий чат и чат чужой компании → 404, 0 вызовов LLM.
func TestGenerateChatSummary_NotFound(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"missing chat", generatePath("999", "company_id=1")},
		{"foreign company chat", generatePath("12", "company_id=1")},
		{"own chat requested by other company", generatePath("10", "company_id=2")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newGenFixture()
			r := f.aiRouter()

			w := doGenRequest(r, http.MethodPost, tc.path, nil)
			if w.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
			}
			assertErrorBody(t, w)
			if got := f.summarizer.callCount(); got != 0 {
				t.Fatalf("expected 0 summarizer calls, got %d", got)
			}
		})
	}
}

// 2A-6: чат без сообщений → 400, 0 вызовов LLM.
func TestGenerateChatSummary_NoMessages_400(t *testing.T) {
	f := newGenFixture()
	r := f.aiRouter()

	w := doGenRequest(r, http.MethodPost, generatePath("11", "company_id=1"), nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	assertErrorBody(t, w)
	if got := f.summarizer.callCount(); got != 0 {
		t.Fatalf("expected 0 summarizer calls, got %d", got)
	}
}

// 2A-7: невалидный :id или force → 400, LLM не вызывается.
func TestGenerateChatSummary_InvalidParams_400(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"id abc", generatePath("abc", "")},
		{"id zero", generatePath("0", "")},
		{"id negative", generatePath("-1", "")},
		{"force 1", generatePath("10", "force=1")},
		{"force yes", generatePath("10", "force=yes")},
		{"force TRUE", generatePath("10", "force=TRUE")},
		{"force empty", generatePath("10", "force=")},
		{"force without value", generatePath("10", "force")},
		{"force repeated", generatePath("10", "force=true&force=false")},
		{"force invalid escape", generatePath("10", "force=%ZZ")},
		{"force valid and invalid escape", generatePath("10", "force=true&force=%ZZ")},
		{"invalid company_id", generatePath("10", "company_id=abc")},
		{"zero company_id", generatePath("10", "company_id=0")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newGenFixture()
			r := f.aiRouter()

			w := doGenRequest(r, http.MethodPost, tc.path, nil)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d, body: %s", tc.path, w.Code, w.Body.String())
			}
			assertErrorBody(t, w)
			if got := f.summarizer.callCount(); got != 0 {
				t.Fatalf("expected 0 summarizer calls, got %d", got)
			}
		})
	}
}

// --- tests: 503 / 502 ---

// 2A-1 / Q5: AI отключён → 503 для существующего чата компании, даже при актуальном кэше.
func TestGenerateChatSummary_AIDisabled_503(t *testing.T) {
	for _, withCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%v", withCache), func(t *testing.T) {
			f := newGenFixture()
			if withCache {
				f.withCachedSummary(genLastMessageID)
			}
			r := f.noAIRouter()

			w := doGenRequest(r, http.MethodPost, generatePath("10", "company_id=1"), nil)
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected 503, got %d, body: %s", w.Code, w.Body.String())
			}
			assertErrorBody(t, w)
			if got := f.summarizer.callCount(); got != 0 {
				t.Fatalf("expected 0 summarizer calls, got %d", got)
			}
			if got := f.summaries.upserts(); got != 0 {
				t.Fatalf("expected 0 upserts, got %d", got)
			}
		})
	}
}

// Q5: при отключённом AI проверка чата/компании идёт раньше → 404 для чужого чата.
func TestGenerateChatSummary_AIDisabled_ForeignChat_404(t *testing.T) {
	f := newGenFixture()
	r := f.noAIRouter()

	w := doGenRequest(r, http.MethodPost, generatePath("12", "company_id=1"), nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
}

// RFC 4.1: summaryUsecase == nil в handler → 503.
func TestGenerateChatSummary_NilUsecase_503(t *testing.T) {
	r := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).InitRoutes()

	w := doGenRequest(r, http.MethodPost, generatePath("10", "company_id=1"), nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d, body: %s", w.Code, w.Body.String())
	}
	assertErrorBody(t, w)
}

// 2A-8: ошибки summarizer → 503 (ErrUnavailable) / 502 (ErrBadGateway); тело без текста ошибки провайдера; запись не меняется.
func TestGenerateChatSummary_SummarizerErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"unavailable", fmt.Errorf("%w: provider said %s", domain.ErrUnavailable, secretMarker), http.StatusServiceUnavailable},
		{"unavailable retryable", fmt.Errorf("%w: %w: %s", domain.ErrUnavailable, usecase.ErrAIRetryable, secretMarker), http.StatusServiceUnavailable},
		{"bad gateway", fmt.Errorf("%w: invalid json %s", domain.ErrBadGateway, secretMarker), http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newGenFixture()
			cached := f.withCachedSummary(101) // устаревший кэш → требуется вызов LLM
			f.summarizer.err = tc.err
			r := f.aiRouter()

			w := doGenRequest(r, http.MethodPost, generatePath("10", "company_id=1"), nil)
			if w.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d, body: %s", tc.wantCode, w.Code, w.Body.String())
			}
			msg := assertErrorBody(t, w)
			if strings.Contains(w.Body.String(), secretMarker) || strings.Contains(msg, "provider") {
				t.Fatalf("response leaks summarizer error: %s", w.Body.String())
			}
			if got := f.summarizer.callCount(); got != 1 {
				t.Fatalf("expected exactly 1 summarizer call (no retries), got %d", got)
			}
			if got := f.summaries.upserts(); got != 0 {
				t.Fatalf("expected 0 upserts, got %d", got)
			}
			if stored := f.summaries.stored[genChatWithMsgs]; stored != cached {
				t.Fatalf("stored summary changed: %+v -> %+v", cached, stored)
			}
		})
	}
}

// 2A-8: невалидный risk из summarizer (usecase → ErrBadGateway) → 502, запись не меняется.
func TestGenerateChatSummary_InvalidRisk_502(t *testing.T) {
	f := newGenFixture()
	f.summarizer.res = &usecase.AIAnalysisResult{Topic: "t", Risk: domain.RiskLevel("critical")}
	r := f.aiRouter()

	w := doGenRequest(r, http.MethodPost, generatePath("10", "company_id=1"), nil)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d, body: %s", w.Code, w.Body.String())
	}
	assertErrorBody(t, w)
	if got := f.summaries.upserts(); got != 0 {
		t.Fatalf("expected 0 upserts, got %d", got)
	}
}

// --- 2A-14: регрессия старых ручек ---

func TestChatSummaryLegacyRoutes_Unchanged(t *testing.T) {
	t.Run("GET existing summary 200", func(t *testing.T) {
		f := newGenFixture()
		cached := f.withCachedSummary(genLastMessageID)
		r := f.aiRouter()

		w := doGenRequest(r, http.MethodGet, "/chats/10/summary?company_id=1", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
		}
		if s := decodeSummary(t, w); s.Topic != cached.Topic {
			t.Fatalf("unexpected summary: %+v", s)
		}
		if got := f.summarizer.callCount(); got != 0 {
			t.Fatalf("GET must not call summarizer, got %d calls", got)
		}
	})

	t.Run("GET foreign chat 404", func(t *testing.T) {
		f := newGenFixture()
		r := f.aiRouter()

		w := doGenRequest(r, http.MethodGet, "/chats/12/summary?company_id=1", nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("GET missing summary 404", func(t *testing.T) {
		f := newGenFixture()
		r := f.aiRouter()

		w := doGenRequest(r, http.MethodGet, "/chats/10/summary?company_id=1", nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("POST manual upsert 200", func(t *testing.T) {
		f := newGenFixture()
		r := f.aiRouter()

		body := []byte(`{"topic":"manual","risk":"high","last_message_id":102}`)
		w := doGenRequest(r, http.MethodPost, "/chats/10/summary?company_id=1", body)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"saved"`) {
			t.Fatalf("expected status saved, got %s", w.Body.String())
		}
		if got := f.summarizer.callCount(); got != 0 {
			t.Fatalf("POST /summary must not call summarizer, got %d calls", got)
		}
		if stored := f.summaries.stored[genChatWithMsgs]; stored.Topic != "manual" || stored.Risk != domain.RiskHigh {
			t.Fatalf("unexpected stored summary: %+v", stored)
		}
	})

	t.Run("POST foreign chat 404", func(t *testing.T) {
		f := newGenFixture()
		r := f.aiRouter()

		w := doGenRequest(r, http.MethodPost, "/chats/12/summary?company_id=1", []byte(`{"topic":"x"}`))
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("nil usecase keeps 500", func(t *testing.T) {
		r := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).InitRoutes()

		w := doGenRequest(r, http.MethodGet, "/chats/10/summary?company_id=1", nil)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 for GET with nil usecase, got %d", w.Code)
		}
	})
}
