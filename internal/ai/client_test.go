package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

// Все тесты пакета используют только loopback httptest.Server или заведомо закрытый
// локальный адрес; внешняя сеть и БД не нужны.

const (
	testAPIKey     = "sk-test-SECRET-KEY-0123456789"
	testModel      = "test-model"
	providerMarker = "PROVIDER-BODY-MARKER-7f3a9c"
	testTimeout    = 5 * time.Second
	testMaxChars   = 4000
)

// capturedRequest — запрос, перехваченный тестовым сервером.
type capturedRequest struct {
	Method        string
	Path          string
	Authorization string
	ContentType   string
	Accept        string
	Body          []byte
}

type stubServer struct {
	srv      *httptest.Server
	requests chan capturedRequest
	calls    atomic.Int32
}

// newStubServer поднимает httptest.Server, который сохраняет каждый запрос и отвечает через respond.
// Запрос попадает в канал до ответа, поэтому после возврата AnalyzeChat он гарантированно доступен.
func newStubServer(t *testing.T, respond http.HandlerFunc) *stubServer {
	t.Helper()
	s := &stubServer{requests: make(chan capturedRequest, 16)}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("stub server: read request body: %v", err)
		}
		select {
		case s.requests <- capturedRequest{
			Method:        r.Method,
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			ContentType:   r.Header.Get("Content-Type"),
			Accept:        r.Header.Get("Accept"),
			Body:          body,
		}:
		default:
			t.Errorf("stub server: unexpected number of requests")
		}
		respond(w, r)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubServer) lastRequest(t *testing.T) capturedRequest {
	t.Helper()
	select {
	case r := <-s.requests:
		return r
	default:
		t.Fatal("stub server: no request captured")
		return capturedRequest{}
	}
}

// respondWith отвечает статусом, заголовками и телом.
func respondWith(status int, body string, header map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for k, v := range header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// envelope упаковывает content в OpenAI-ответ chat completions.
func envelope(t *testing.T, content string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"id": "chatcmpl-test",
		"choices": []any{map[string]any{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": content},
			"finish_reason": "stop",
		}},
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return string(b)
}

func newTestClient(t *testing.T, baseURL string, timeout time.Duration, maxChars int) *Client {
	t.Helper()
	c, err := NewClient(baseURL, testAPIKey, testModel, timeout, maxChars)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func testInput() usecase.AnalyzeChatInput {
	base := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	return usecase.AnalyzeChatInput{Messages: []domain.Message{
		{ID: 1, ChatID: 7, Direction: domain.MessageDirectionIn, Text: "Здравствуйте, нужны трубы", SentAt: base},
		{ID: 2, ChatID: 7, Direction: domain.MessageDirectionOut, Text: "Добрый день, пришлю КП", SentAt: base.Add(5 * time.Minute)},
	}}
}

type errClass struct {
	unavailable bool
	retryable   bool
	badGateway  bool
}

func assertErrClass(t *testing.T, err error, want errClass) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := errors.Is(err, domain.ErrUnavailable); got != want.unavailable {
		t.Errorf("errors.Is(err, ErrUnavailable) = %v, want %v (err: %v)", got, want.unavailable, err)
	}
	if got := errors.Is(err, usecase.ErrAIRetryable); got != want.retryable {
		t.Errorf("errors.Is(err, ErrAIRetryable) = %v, want %v (err: %v)", got, want.retryable, err)
	}
	if got := errors.Is(err, domain.ErrBadGateway); got != want.badGateway {
		t.Errorf("errors.Is(err, ErrBadGateway) = %v, want %v (err: %v)", got, want.badGateway, err)
	}
}

// assertNoLeak проверяет, что ни ключ API, ни тело ответа провайдера не попали в текст ошибки.
func assertNoLeak(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	if strings.Contains(msg, testAPIKey) {
		t.Errorf("error text leaks API key: %q", msg)
	}
	if strings.Contains(msg, providerMarker) {
		t.Errorf("error text leaks provider response body: %q", msg)
	}
}

// syncBuffer — потокобезопасный приёмник стандартного логгера.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLog перенаправляет стандартный логгер на время теста. Тесты пакета не используют t.Parallel.
func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return buf
}

// --- NewClient ---

func TestNewClient_Valid(t *testing.T) {
	tests := []struct {
		name         string
		baseURL      string
		wantEndpoint string
	}{
		{"empty base uses default", "", DefaultBaseURL + "/chat/completions"},
		{"blank base uses default", "   ", DefaultBaseURL + "/chat/completions"},
		{"trailing slash trimmed", "http://127.0.0.1:9/v1/", "http://127.0.0.1:9/v1/chat/completions"},
		{"several trailing slashes trimmed", "https://llm.local/api//", "https://llm.local/api/chat/completions"},
		{"no path", "http://127.0.0.1:9", "http://127.0.0.1:9/chat/completions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.baseURL, "  "+testAPIKey+"  ", " "+testModel+" ", time.Second, 100)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if c.endpoint != tt.wantEndpoint {
				t.Errorf("endpoint = %q, want %q", c.endpoint, tt.wantEndpoint)
			}
			if c.apiKey != testAPIKey || c.model != testModel {
				t.Errorf("key/model not trimmed: model=%q", c.model)
			}
			if c.httpClient.Timeout != time.Second {
				t.Errorf("http timeout = %s, want 1s", c.httpClient.Timeout)
			}
			if c.maxChars != 100 {
				t.Errorf("maxChars = %d, want 100", c.maxChars)
			}
		})
	}
}

func TestNewClient_InvalidArguments(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		apiKey   string
		model    string
		timeout  time.Duration
		maxChars int
	}{
		{"empty key", "", "", testModel, time.Second, 100},
		{"blank key", "", "   ", testModel, time.Second, 100},
		{"empty model", "", testAPIKey, "", time.Second, 100},
		{"blank model", "", testAPIKey, " \t", time.Second, 100},
		{"zero timeout", "", testAPIKey, testModel, 0, 100},
		{"negative timeout", "", testAPIKey, testModel, -time.Second, 100},
		{"zero max chars", "", testAPIKey, testModel, time.Second, 0},
		{"negative max chars", "", testAPIKey, testModel, time.Second, -1},
		{"unsupported scheme", "ftp://llm.local/v1", testAPIKey, testModel, time.Second, 100},
		{"no scheme", "llm.local/v1", testAPIKey, testModel, time.Second, 100},
		{"no host", "http:///v1", testAPIKey, testModel, time.Second, 100},
		{"unparsable", "http://[::1", testAPIKey, testModel, time.Second, 100},
		{"credentials in URL", "http://user:" + testAPIKey + "@llm.local/v1", testAPIKey, testModel, time.Second, 100},
		{"query in URL", "http://llm.local/v1?key=" + testAPIKey, testAPIKey, testModel, time.Second, 100},
		{"fragment in URL", "http://llm.local/v1#frag", testAPIKey, testModel, time.Second, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.baseURL, tt.apiKey, tt.model, tt.timeout, tt.maxChars)
			if err == nil {
				t.Fatalf("NewClient succeeded, want error (client: %v)", c != nil)
			}
			if c != nil {
				t.Error("NewClient returned non-nil client with error")
			}
			if strings.Contains(err.Error(), testAPIKey) {
				t.Errorf("error text leaks API key: %q", err.Error())
			}
		})
	}
}

// --- Запрос ---

func TestAnalyzeChat_RequestFormat(t *testing.T) {
	s := newStubServer(t, respondWith(http.StatusOK, envelope(t, analysisJSON(t, nil)), nil))
	c := newTestClient(t, s.srv.URL+"/v1/", testTimeout, testMaxChars)

	if _, err := c.AnalyzeChat(context.Background(), testInput()); err != nil {
		t.Fatalf("AnalyzeChat: %v", err)
	}
	req := s.lastRequest(t)

	if req.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", req.Method)
	}
	if req.Path != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", req.Path)
	}
	if req.Authorization != "Bearer "+testAPIKey {
		t.Error("Authorization header is not \"Bearer <key>\"")
	}
	if !strings.HasPrefix(req.ContentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", req.ContentType)
	}
	if req.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", req.Accept)
	}

	body := decodeSentRequest(t, req)
	if body.Model != testModel {
		t.Errorf("model = %q, want %q", body.Model, testModel)
	}
	if body.Temperature == nil {
		t.Error("temperature is missing")
	}
	if len(body.Messages) != 2 {
		t.Fatalf("messages count = %d, want 2", len(body.Messages))
	}
	if body.Messages[0].Role != "system" || body.Messages[1].Role != "user" {
		t.Errorf("roles = %q, %q; want system, user", body.Messages[0].Role, body.Messages[1].Role)
	}
	sys := body.Messages[0].Content
	for _, want := range []string{
		"JSON", "low, medium, high",
		"new_lead, negotiation, waiting_reply, interested, unhappy, lost, action_required",
		"complaint, refusal, negative, urgent", "no_reply",
	} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt does not contain %q", want)
		}
	}
	if strings.Contains(sys, testAPIKey) || strings.Contains(body.Messages[1].Content, testAPIKey) {
		t.Error("API key leaked into request messages")
	}
}

func TestAnalyzeChat_EmptyMessagesNoNetworkCall(t *testing.T) {
	s := newStubServer(t, respondWith(http.StatusOK, envelope(t, analysisJSON(t, nil)), nil))
	c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)

	for _, in := range []usecase.AnalyzeChatInput{{}, {Messages: []domain.Message{}}} {
		res, err := c.AnalyzeChat(context.Background(), in)
		if res != nil {
			t.Error("result must be nil on error")
		}
		if !errors.Is(err, domain.ErrValidation) {
			t.Errorf("err = %v, want ErrValidation", err)
		}
	}
	if n := s.calls.Load(); n != 0 {
		t.Errorf("provider called %d times, want 0", n)
	}
}

// --- Классификация HTTP-статусов (2A-8, 2A-13, Q7) ---

func TestAnalyzeChat_RateLimited(t *testing.T) {
	fixedNow := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		retryAfter string // "" — заголовка нет
		want       time.Duration
	}{
		{"no Retry-After", "", -time.Second},
		{"Retry-After seconds", "3", 3 * time.Second},
		{"Retry-After seconds with spaces", " 7 ", 7 * time.Second},
		{"Retry-After zero", "0", 0},
		{"Retry-After large seconds", "120", 120 * time.Second},
		{"Retry-After HTTP-date in future", fixedNow.Add(9 * time.Second).Format(http.TimeFormat), 9 * time.Second},
		{"Retry-After HTTP-date at current time", fixedNow.Format(http.TimeFormat), 0},
		{"Retry-After HTTP-date in past", fixedNow.Add(-time.Minute).Format(http.TimeFormat), -time.Second},
		{"Retry-After garbage", "soon", -time.Second},
		{"Retry-After negative", "-5", -time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := map[string]string{}
			if tt.retryAfter != "" {
				header["Retry-After"] = tt.retryAfter
			}
			body := `{"error":{"message":"rate limited ` + providerMarker + `"}}`
			s := newStubServer(t, respondWith(http.StatusTooManyRequests, body, header))
			c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)
			c.now = func() time.Time { return fixedNow }

			res, err := c.AnalyzeChat(context.Background(), testInput())
			if res != nil {
				t.Error("result must be nil on error")
			}
			assertErrClass(t, err, errClass{unavailable: true, retryable: true})
			assertNoLeak(t, err)

			var ra usecase.RetryAfterError
			if !errors.As(err, &ra) {
				t.Fatalf("429 error does not implement usecase.RetryAfterError: %v", err)
			}
			if got := ra.RetryAfter(); got != tt.want {
				t.Errorf("RetryAfter() = %s, want %s", got, tt.want)
			}
			if n := s.calls.Load(); n != 1 {
				t.Errorf("provider called %d times, want 1 (client must not retry)", n)
			}
		})
	}
}

// 2A-8, 2A-13, Q7: любой 5xx провайдера → ErrUnavailable (503) + ErrAIRetryable,
// реализует RetryAfterError с разобранным Retry-After. Сам клиент не повторяет.
func TestAnalyzeChat_ServerErrorIsUnavailableRetryable(t *testing.T) {
	fixedNow := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	codes := []int{
		http.StatusInternalServerError, http.StatusNotImplemented, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout, 599,
	}
	headers := []struct {
		name       string
		retryAfter string // "" — заголовка нет
		want       time.Duration
	}{
		{"no Retry-After", "", -time.Second},
		{"Retry-After seconds", "4", 4 * time.Second},
		{"Retry-After zero", "0", 0},
		{"Retry-After large seconds", "120", 120 * time.Second},
		{"Retry-After HTTP-date", fixedNow.Add(6 * time.Second).Format(http.TimeFormat), 6 * time.Second},
		{"Retry-After garbage", "later", -time.Second},
		{"Retry-After negative", "-5", -time.Second},
	}
	for _, code := range codes {
		for _, h := range headers {
			t.Run(strconv.Itoa(code)+" "+h.name, func(t *testing.T) {
				header := map[string]string{}
				if h.retryAfter != "" {
					header["Retry-After"] = h.retryAfter
				}
				body := `{"error":"` + providerMarker + ` ` + testAPIKey + `"}`
				s := newStubServer(t, respondWith(code, body, header))
				c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)
				c.now = func() time.Time { return fixedNow }

				res, err := c.AnalyzeChat(context.Background(), testInput())
				if res != nil {
					t.Error("result must be nil on error")
				}
				assertErrClass(t, err, errClass{unavailable: true, retryable: true})
				assertNoLeak(t, err)
				if !strings.Contains(err.Error(), "status "+strconv.Itoa(code)) {
					t.Errorf("error text must contain the status code %d: %q", code, err.Error())
				}
				var ra usecase.RetryAfterError
				if !errors.As(err, &ra) {
					t.Fatalf("5xx error does not implement usecase.RetryAfterError: %v", err)
				}
				if got := ra.RetryAfter(); got != h.want {
					t.Errorf("RetryAfter() = %s, want %s", got, h.want)
				}
				if n := s.calls.Load(); n != 1 {
					t.Errorf("provider called %d times, want 1 (client must not retry)", n)
				}
			})
		}
	}
}

// Статусы вне 2xx, 429 и 5xx — неожиданный ответ провайдера → ErrBadGateway без retryable.
func TestAnalyzeChat_UnexpectedStatusOutside5xxIsBadGateway(t *testing.T) {
	for _, code := range []int{http.StatusNotModified, http.StatusMultipleChoices, 499, 600} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			s := newStubServer(t, respondWith(code, `{"error":"`+providerMarker+`"}`, map[string]string{"Retry-After": "1"}))
			c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)

			res, err := c.AnalyzeChat(context.Background(), testInput())
			if res != nil {
				t.Error("result must be nil on error")
			}
			assertErrClass(t, err, errClass{badGateway: true})
			assertNoLeak(t, err)
			var ra usecase.RetryAfterError
			if errors.As(err, &ra) {
				t.Errorf("status %d error must not implement RetryAfterError", code)
			}
		})
	}
}

func TestAnalyzeChat_ClientErrorIsBadGateway(t *testing.T) {
	for _, code := range []int{
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
		http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity,
	} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			body := `{"error":{"message":"invalid api key ` + testAPIKey + ` ` + providerMarker + `"}}`
			s := newStubServer(t, respondWith(code, body, nil))
			c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)

			res, err := c.AnalyzeChat(context.Background(), testInput())
			if res != nil {
				t.Error("result must be nil on error")
			}
			assertErrClass(t, err, errClass{badGateway: true})
			assertNoLeak(t, err)
		})
	}
}

func TestAnalyzeChat_RedirectNotFollowed(t *testing.T) {
	target := newStubServer(t, respondWith(http.StatusOK, envelope(t, analysisJSON(t, nil)), nil))
	s := newStubServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.srv.URL+"/v1/chat/completions", http.StatusTemporaryRedirect)
	})
	c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)

	_, err := c.AnalyzeChat(context.Background(), testInput())
	assertErrClass(t, err, errClass{badGateway: true})
	assertNoLeak(t, err)
	if n := target.calls.Load(); n != 0 {
		t.Errorf("redirect target called %d times, want 0 (request with key must not be forwarded)", n)
	}
}

// --- Таймауты, отмена и сетевые ошибки (2A-8, 2A-13, Q7) ---

// newBlockingServer поднимает сервер, обработчик которого ждёт отмены запроса клиентом
// или закрытия release в t.Cleanup, чтобы под -race не оставалось висящих горутин.
// Cleanup закрытия release регистрируется после srv.Close и поэтому выполняется раньше него.
func newBlockingServer(t *testing.T, flushHeaders bool) *stubServer {
	t.Helper()
	release := make(chan struct{})
	s := newStubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if flushHeaders {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"choices":[`)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	return s
}

func TestAnalyzeChat_ClientTimeoutIsUnavailableNotRetryable(t *testing.T) {
	tests := []struct {
		name         string
		flushHeaders bool
	}{
		{"no response headers", false},
		{"body stalls after headers", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newBlockingServer(t, tt.flushHeaders)
			c := newTestClient(t, s.srv.URL, 50*time.Millisecond, testMaxChars)

			res, err := c.AnalyzeChat(context.Background(), testInput())
			if res != nil {
				t.Error("result must be nil on error")
			}
			assertErrClass(t, err, errClass{unavailable: true})
			assertNoLeak(t, err)
		})
	}
}

func TestAnalyzeChat_ContextDeadlineIsUnavailableNotRetryable(t *testing.T) {
	s := newBlockingServer(t, false)
	c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	res, err := c.AnalyzeChat(ctx, testInput())
	if res != nil {
		t.Error("result must be nil on error")
	}
	assertErrClass(t, err, errClass{unavailable: true})
	assertNoLeak(t, err)
}

func TestAnalyzeChat_ContextCanceledIsUnavailableNotRetryable(t *testing.T) {
	s := newStubServer(t, respondWith(http.StatusOK, envelope(t, analysisJSON(t, nil)), nil))
	c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := c.AnalyzeChat(ctx, testInput())
	if res != nil {
		t.Error("result must be nil on error")
	}
	assertErrClass(t, err, errClass{unavailable: true})
	assertNoLeak(t, err)
}

func TestAnalyzeChat_ConnectionRefusedIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := srv.URL
	srv.Close() // адрес на loopback заведомо закрыт

	c := newTestClient(t, closedURL, 10*time.Second, testMaxChars)
	res, err := c.AnalyzeChat(context.Background(), testInput())
	if res != nil {
		t.Error("result must be nil on error")
	}
	assertErrClass(t, err, errClass{unavailable: true, retryable: true})
	assertNoLeak(t, err)
}

func TestAnalyzeChat_ConnectionDroppedIsRetryable(t *testing.T) {
	s := newStubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("response writer does not support hijacking")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = conn.Close()
	})
	c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)

	res, err := c.AnalyzeChat(context.Background(), testInput())
	if res != nil {
		t.Error("result must be nil on error")
	}
	assertErrClass(t, err, errClass{unavailable: true, retryable: true})
	assertNoLeak(t, err)
}

// Q7: обрыв соединения посреди тела 200-ответа → ErrUnavailable без retryable-маркера:
// провайдер уже принял запрос и мог его обработать и оплатить.
func TestAnalyzeChat_BodyDroppedAfter2xxIsUnavailableNotRetryable(t *testing.T) {
	s := newStubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("response writer does not support hijacking")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 4096\r\n\r\n" +
			`{"choices":[{"message":{"content":"` + providerMarker)
		_ = buf.Flush()
	})
	c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)

	res, err := c.AnalyzeChat(context.Background(), testInput())
	if res != nil {
		t.Error("result must be nil on error")
	}
	assertErrClass(t, err, errClass{unavailable: true})
	assertNoLeak(t, err)
	if n := s.calls.Load(); n != 1 {
		t.Errorf("provider called %d times, want 1", n)
	}
}

// TestAnalyzeChat_ErrorsDoNotLeakSecrets: ни ключ, ни тело ответа провайдера не попадают
// в текст ошибки, в том числе когда провайдер возвращает их в теле.
func TestAnalyzeChat_ErrorsDoNotLeakSecrets(t *testing.T) {
	leaky := `{"error":{"message":"` + providerMarker + `","key":"` + testAPIKey + `"}}`
	tests := []struct {
		name   string
		status int
		body   string
		want   errClass
	}{
		{"400", http.StatusBadRequest, leaky, errClass{badGateway: true}},
		{"401", http.StatusUnauthorized, leaky, errClass{badGateway: true}},
		{"429", http.StatusTooManyRequests, leaky, errClass{unavailable: true, retryable: true}},
		{"500", http.StatusInternalServerError, leaky, errClass{unavailable: true, retryable: true}},
		{"503", http.StatusServiceUnavailable, leaky, errClass{unavailable: true, retryable: true}},
		{"200 invalid envelope", http.StatusOK, providerMarker + " " + testAPIKey, errClass{badGateway: true}},
		{"200 content not JSON", http.StatusOK, envelope(t, providerMarker+" "+testAPIKey), errClass{badGateway: true}},
		{"200 invalid risk value", http.StatusOK, envelope(t, analysisJSON(t, func(m map[string]any) {
			m["risk"] = providerMarker
		})), errClass{badGateway: true}},
		{"200 non-string topic", http.StatusOK, envelope(t, analysisJSON(t, func(m map[string]any) {
			m["topic"] = map[string]any{"text": providerMarker}
		})), errClass{badGateway: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captureLog(t)
			s := newStubServer(t, respondWith(tt.status, tt.body, nil))
			c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)

			_, err := c.AnalyzeChat(context.Background(), testInput())
			assertErrClass(t, err, tt.want)
			assertNoLeak(t, err)
		})
	}
}
