// Package ai реализует OpenAI-совместимый клиент AI-анализа диалогов (RFC 02, трек 2A).
//
// Пакет реализует usecase.ChatSummarizer на стандартной библиотеке net/http.
// Ни API-ключ, ни тело ответа провайдера не попадают в ошибки и логи.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

const (
	// DefaultBaseURL используется, если base URL не задан.
	DefaultBaseURL = "https://api.openai.com/v1"

	chatCompletionsPath = "/chat/completions"
	maxResponseBytes    = 1 << 20  // предел чтения успешного ответа провайдера
	maxDrainBytes       = 64 << 10 // предел дочитывания тела ответа с ошибкой
	requestTemperature  = 0.2
	noRetryAfter        = -time.Second
)

var _ usecase.ChatSummarizer = (*Client)(nil)

// Client — OpenAI-совместимый клиент chat completions.
type Client struct {
	endpoint   string
	apiKey     string
	model      string
	maxChars   int
	httpClient *http.Client
	now        func() time.Time
}

// NewClient создаёт клиент. Пустой baseURL заменяется на DefaultBaseURL, хвостовой "/" обрезается.
// timeout задаёт http.Client.Timeout на весь запрос, maxChars — лимит символов (рун) текста сообщений в контексте.
func NewClient(baseURL, apiKey, model string, timeout time.Duration, maxChars int) (*Client, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("ai: api key is required")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, errors.New("ai: model is required")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("ai: timeout must be positive, got %s", timeout)
	}
	if maxChars <= 0 {
		return nil, fmt.Errorf("ai: max chars must be positive, got %d", maxChars)
	}
	base, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		endpoint: base + chatCompletionsPath,
		apiKey:   apiKey,
		model:    model,
		maxChars: maxChars,
		httpClient: &http.Client{
			Timeout: timeout,
			// Редиректы не выполняются: 3xx классифицируется как неожиданный статус (502),
			// а запрос с ключом не уходит на другой адрес.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		now: time.Now,
	}, nil
}

// normalizeBaseURL проверяет base URL. Текст ошибок не содержит сам URL.
func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = DefaultBaseURL
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("ai: invalid base URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("ai: base URL must use http or https scheme")
	}
	if u.Host == "" {
		return "", errors.New("ai: base URL must contain a host")
	}
	if u.User != nil {
		return "", errors.New("ai: base URL must not contain credentials")
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("ai: base URL must not contain query or fragment")
	}
	return strings.TrimRight(raw, "/"), nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

// AnalyzeChat отправляет контекст диалога в LLM и возвращает провалидированный результат.
//
// Классификация ошибок (RFC 02, D5/D7, Q7):
//   - пустой in.Messages → domain.ErrValidation, сетевого вызова нет;
//   - таймаут (клиента или ctx) и отмена ctx → domain.ErrUnavailable без retryable-маркера;
//   - прочие сетевые ошибки до получения ответа → domain.ErrUnavailable + usecase.ErrAIRetryable;
//   - обрыв при чтении тела 2xx-ответа → domain.ErrUnavailable без retryable-маркера;
//   - 429 и любой 5xx (500–599) → domain.ErrUnavailable + usecase.ErrAIRetryable,
//     реализуют usecase.RetryAfterError с разобранным Retry-After (повтор в фоне по Q7);
//   - иные не-2xx (1xx, 3xx, 4xx кроме 429, 600+) и невалидный ответ → domain.ErrBadGateway.
func (c *Client) AnalyzeChat(ctx context.Context, in usecase.AnalyzeChatInput) (*usecase.AIAnalysisResult, error) {
	if len(in.Messages) == 0 {
		return nil, fmt.Errorf("%w: ai: no messages to analyze", domain.ErrValidation)
	}

	payload, err := json.Marshal(chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: buildUserContent(in, c.maxChars)},
		},
		Temperature: requestTemperature,
	})
	if err != nil {
		return nil, fmt.Errorf("ai: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ai: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, classifyTransportError(ctx, err)
	}
	defer resp.Body.Close()

	if err := statusError(resp, c.now()); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, classifyBodyReadError(ctx, err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("%w: ai response exceeds %d bytes", domain.ErrBadGateway, maxResponseBytes)
	}
	return parseCompletion(body)
}

// classifyTransportError переводит ошибку транспорта в доменную. Текст исходной ошибки
// не включается: в ней может оказаться URL или иные детали запроса.
func classifyTransportError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.Canceled) {
			return fmt.Errorf("%w: ai request canceled", domain.ErrUnavailable)
		}
		return fmt.Errorf("%w: ai request timed out", domain.ErrUnavailable)
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return fmt.Errorf("%w: ai request timed out", domain.ErrUnavailable)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: ai request canceled", domain.ErrUnavailable)
	}
	return fmt.Errorf("%w: %w: ai request failed (%s)", domain.ErrUnavailable, usecase.ErrAIRetryable, describeNetworkError(err))
}

// classifyBodyReadError переводит ошибку чтения тела успешного (2xx) ответа в доменную.
// Провайдер уже принял запрос и мог его обработать и оплатить, поэтому, как и таймаут, это
// domain.ErrUnavailable без retryable-маркера (Q7). Таймаут и отмена классифицируются как обычно.
func classifyBodyReadError(ctx context.Context, err error) error {
	classified := classifyTransportError(ctx, err)
	if !errors.Is(classified, usecase.ErrAIRetryable) {
		return classified
	}
	return fmt.Errorf("%w: ai response body read failed (%s)", domain.ErrUnavailable, describeNetworkError(err))
}

func describeNetworkError(err error) string {
	var opErr *net.OpError
	switch {
	case errors.As(err, &opErr) && opErr.Op != "":
		return "network error, op " + opErr.Op
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "connection closed by provider"
	default:
		return "network error"
	}
}

// statusError возвращает nil для 2xx, иначе дочитывает (без сохранения) и классифицирует ответ.
func statusError(resp *http.Response, now time.Time) error {
	code := resp.StatusCode
	if code >= 200 && code < 300 {
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes)) // тело ошибки провайдера не используется
	switch {
	case code == http.StatusTooManyRequests, code >= 500 && code <= 599:
		// Внешний сервис временно недоступен → 503. По Q7 429 и любой 5xx повторяются в фоне
		// (5xx почти всегда временный, токены за него не списываются) с учётом Retry-After.
		return &providerStatusError{code: code, retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), now)}
	default:
		return fmt.Errorf("%w: ai provider returned unexpected status %d", domain.ErrBadGateway, code)
	}
}

// providerStatusError — временная ошибка провайдера: ответ 429 или 5xx.
// Оборачивает domain.ErrUnavailable и usecase.ErrAIRetryable, реализует usecase.RetryAfterError.
// Текст содержит только статус-код и Retry-After, без тела ответа и ключа.
type providerStatusError struct {
	code       int
	retryAfter time.Duration
}

var _ usecase.RetryAfterError = (*providerStatusError)(nil)

func (e *providerStatusError) Error() string {
	msg := fmt.Sprintf("%v: %v: ai provider returned status %d", domain.ErrUnavailable, usecase.ErrAIRetryable, e.code)
	if e.retryAfter > 0 {
		msg += fmt.Sprintf(" (retry after %s)", e.retryAfter)
	}
	return msg
}

func (e *providerStatusError) Unwrap() []error {
	return []error{domain.ErrUnavailable, usecase.ErrAIRetryable}
}

// RetryAfter — неотрицательная задержка из заголовка или отрицательное значение без подсказки.
// Ограничение сверху применяет usecase.
func (e *providerStatusError) RetryAfter() time.Duration {
	return e.retryAfter
}

// parseRetryAfter разбирает Retry-After: целые секунды или HTTP-date.
// Отсутствие, некорректное значение и дата в прошлом → noRetryAfter.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return noRetryAfter
	}
	if isASCIIDigits(v) {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n > math.MaxInt64/int64(time.Second) {
			return time.Duration(math.MaxInt64) // переполнение: заведомо больше любого допустимого лимита
		}
		return time.Duration(n) * time.Second
	}
	t, err := http.ParseTime(v)
	if err != nil {
		return noRetryAfter
	}
	if d := t.Sub(now); d >= 0 {
		return d
	}
	return noRetryAfter
}

func isASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}
