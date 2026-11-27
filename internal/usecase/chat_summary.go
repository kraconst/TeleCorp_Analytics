package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

const (
	// summaryQueueSize — размер буфера фоновой очереди генерации (Q7).
	summaryQueueSize = 100
	// summaryRetryBackoff — пауза перед единственным повтором без валидного Retry-After (Q7).
	summaryRetryBackoff = 2 * time.Second
	// summaryMaxRetryAfter — максимальный учитываемый Retry-After провайдера (Q7).
	summaryMaxRetryAfter = 10 * time.Second
	// defaultSummaryMaxMessages — лимит сообщений контекста при maxMessages <= 0.
	defaultSummaryMaxMessages = 50
)

// AIAnalysisHandler обрабатывает результаты AI-анализа (автостатусы и алерты) и адаптирует функцию к ChatAnalysisHandler.
type AIAnalysisHandler func(ctx context.Context, companyID, chatID int64, result *AIAnalysisResult) error

func (fn AIAnalysisHandler) HandleAnalysis(ctx context.Context, companyID, chatID int64, res AIAnalysisResult) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, companyID, chatID, &res)
}

// summaryJob — задача фоновой генерации саммари.
type summaryJob struct {
	companyID int64
	chatID    int64
}

type ChatSummaryUsecase struct {
	summaries ChatSummaryRepository
	chats     ChatRepository
	accounts  AccountRepository

	messages    RecentMessageRepository
	summarizer  ChatSummarizer // nil — AI отключён
	analysis    ChatAnalysisHandler
	maxMessages int

	queue  chan summaryJob
	mu     sync.Mutex
	queued map[int64]struct{} // chatID задач, стоящих в очереди
}

// NewChatSummaryUsecase создаёт usecase без AI: Generate возвращает ErrUnavailable, Enqueue — no-op.
func NewChatSummaryUsecase(summaries ChatSummaryRepository, chats ChatRepository, accounts AccountRepository) *ChatSummaryUsecase {
	return NewChatSummaryUsecaseWithAI(summaries, chats, accounts, nil, nil, nil, 0)
}

// NewChatSummaryUsecaseWithAI создаёт usecase с AI-генерацией.
// summarizer == nil — AI отключён; analysis == nil — результат анализа не применяется;
// maxMessages <= 0 — используется лимит по умолчанию (50).
func NewChatSummaryUsecaseWithAI(
	summaries ChatSummaryRepository,
	chats ChatRepository,
	accounts AccountRepository,
	messages RecentMessageRepository,
	summarizer ChatSummarizer,
	analysis ChatAnalysisHandler,
	maxMessages int,
) *ChatSummaryUsecase {
	if maxMessages <= 0 {
		maxMessages = defaultSummaryMaxMessages
	}
	// Typed-nil (например, (*ai.Client)(nil)) приводится к литеральному nil,
	// чтобы проверки `== nil` надёжно означали «зависимость отсутствует».
	if isNilDependency(messages) {
		messages = nil
	}
	if isNilDependency(summarizer) {
		summarizer = nil
	}
	if isNilDependency(analysis) {
		analysis = nil
	}
	return &ChatSummaryUsecase{
		summaries:   summaries,
		chats:       chats,
		accounts:    accounts,
		messages:    messages,
		summarizer:  summarizer,
		analysis:    analysis,
		maxMessages: maxMessages,
		queue:       make(chan summaryJob, summaryQueueSize),
		queued:      make(map[int64]struct{}),
	}
}

func isNilDependency(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

func (u *ChatSummaryUsecase) aiEnabled() bool {
	return u.summarizer != nil && u.messages != nil
}

// SetAnalysisHandler регистрирует обработчик результата AI-анализа.
func (u *ChatSummaryUsecase) SetAnalysisHandler(fn AIAnalysisHandler) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if fn == nil {
		u.analysis = nil
		return
	}
	u.analysis = fn
}

func (u *ChatSummaryUsecase) getAnalysisHandler() ChatAnalysisHandler {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.analysis
}

// ApplyAnalysisResult передаёт результат AI-анализа зарегистрированному обработчику автостатусов и алертов.
func (u *ChatSummaryUsecase) ApplyAnalysisResult(ctx context.Context, companyID, chatID int64, result *AIAnalysisResult) error {
	if companyID <= 0 {
		return fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if chatID <= 0 {
		return fmt.Errorf("%w: invalid chat_id", domain.ErrValidation)
	}
	if result == nil {
		return fmt.Errorf("%w: analysis result is required", domain.ErrValidation)
	}
	handler := u.getAnalysisHandler()
	if handler == nil {
		return nil
	}
	return handler.HandleAnalysis(ctx, companyID, chatID, *result)
}

func (u *ChatSummaryUsecase) Upsert(ctx context.Context, companyID int64, summary domain.ChatSummary) error {
	if companyID <= 0 {
		return fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if summary.ChatID <= 0 {
		return fmt.Errorf("%w: invalid chat_id", domain.ErrValidation)
	}
	if summary.LastMessageID < 0 {
		return fmt.Errorf("%w: invalid last_message_id", domain.ErrValidation)
	}
	if u.chats != nil {
		chat, err := u.chats.GetByID(ctx, summary.ChatID)
		if err != nil {
			return err
		}
		if u.accounts != nil {
			acc, err := u.accounts.GetByID(ctx, chat.AccountID)
			if err != nil {
				return err
			}
			if acc.CompanyID != companyID {
				return fmt.Errorf("%w: chat not found", domain.ErrNotFound)
			}
		}
	}

	summary.Topic = strings.TrimSpace(summary.Topic)
	summary.Agreements = strings.TrimSpace(summary.Agreements)
	summary.NextStep = strings.TrimSpace(summary.NextStep)
	summary.OpenQuestions = strings.TrimSpace(summary.OpenQuestions)
	if summary.Risk == "" {
		summary.Risk = domain.RiskLow
	} else {
		switch summary.Risk {
		case domain.RiskLow, domain.RiskMedium, domain.RiskHigh:
		default:
			return fmt.Errorf("%w: invalid risk '%s'", domain.ErrValidation, summary.Risk)
		}
	}
	summary.GeneratedAt = time.Now().UTC()

	return u.summaries.Upsert(ctx, summary)
}

func (u *ChatSummaryUsecase) GetByChatID(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if chatID <= 0 {
		return nil, fmt.Errorf("%w: invalid chat_id", domain.ErrValidation)
	}
	if u.chats != nil {
		chat, err := u.chats.GetByID(ctx, chatID)
		if err != nil {
			return nil, err
		}
		if u.accounts != nil {
			acc, err := u.accounts.GetByID(ctx, chat.AccountID)
			if err != nil {
				return nil, err
			}
			if acc.CompanyID != companyID {
				return nil, fmt.Errorf("%w: chat not found", domain.ErrNotFound)
			}
		}
	}
	return u.getStoredSummary(ctx, companyID, chatID)
}

func (u *ChatSummaryUsecase) getStoredSummary(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error) {
	summary, err := u.summaries.GetByChatID(ctx, companyID, chatID)
	if err != nil || summary == nil {
		return summary, err
	}
	// Репозиторий может вернуть момент времени в локальной зоне.
	result := *summary
	result.GeneratedAt = result.GeneratedAt.UTC()
	return &result, nil
}

// Generate строит и сохраняет AI-саммари чата компании (RFC 02, порядок Generate / Q5).
// Повторов нет; при актуальном кэше (и !force) возвращается сохранённое саммари без вызова LLM.
func (u *ChatSummaryUsecase) Generate(ctx context.Context, companyID, chatID int64, force bool) (*domain.ChatSummary, error) {
	// 1. Валидация ID.
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if chatID <= 0 {
		return nil, fmt.Errorf("%w: invalid chat_id", domain.ErrValidation)
	}

	// 2. Чат и компания. Без репозиториев проверку пропускать нельзя — это ошибка конфигурации (500).
	if u.summaries == nil || u.chats == nil || u.accounts == nil {
		return nil, errors.New("chat summary usecase: summary, chat or account repository is not configured")
	}
	chat, err := u.chats.GetByID(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("get chat: %w", err)
	}
	if chat == nil {
		return nil, fmt.Errorf("%w: chat not found", domain.ErrNotFound)
	}
	acc, err := u.accounts.GetByID(ctx, chat.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get chat account: %w", err)
	}
	if acc == nil || acc.CompanyID != companyID {
		return nil, fmt.Errorf("%w: chat not found", domain.ErrNotFound)
	}

	// 3. AI отключён — всегда 503, даже при актуальном кэше (Q5).
	if !u.aiEnabled() {
		return nil, fmt.Errorf("%w: ai summarizer is disabled", domain.ErrUnavailable)
	}

	// 4. Последние сообщения (ASC); последний элемент — «последнее сообщение» для кэша (Q3).
	msgs, err := u.messages.ListLatestByChat(ctx, companyID, chatID, u.maxMessages)
	if err != nil {
		return nil, fmt.Errorf("list latest messages: %w", err)
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("%w: chat has no messages", domain.ErrValidation)
	}
	lastMessageID := msgs[len(msgs)-1].ID

	// 5. Кэш по last_message_id. ErrNotFound — саммари ещё нет.
	previous, err := u.getStoredSummary(ctx, companyID, chatID)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("get chat summary: %w", err)
		}
		previous = nil
	}
	if !force && previous != nil && previous.LastMessageID == lastMessageID {
		return previous, nil
	}

	// 6. AI-анализ. Ошибка возвращается без сохранения.
	res, err := u.summarizer.AnalyzeChat(ctx, AnalyzeChatInput{Messages: msgs, Previous: previous})
	if err != nil {
		return nil, fmt.Errorf("analyze chat: %w", err)
	}
	if res == nil {
		return nil, fmt.Errorf("%w: ai summarizer returned empty result", domain.ErrBadGateway)
	}

	// 7. Повторная валидация Risk без подстановки значения по умолчанию.
	switch res.Risk {
	case domain.RiskLow, domain.RiskMedium, domain.RiskHigh:
	default:
		return nil, fmt.Errorf("%w: ai summarizer returned invalid risk", domain.ErrBadGateway)
	}

	// 8. Сохранение напрямую в репозиторий (u.Upsert подставляет RiskLow и меняет GeneratedAt).
	// generated_at приводится к микросекундной точности timestamptz до записи и возврата.
	summary := domain.ChatSummary{
		ChatID:        chatID,
		Topic:         res.Topic,
		Agreements:    res.Agreements,
		NextStep:      res.NextStep,
		OpenQuestions: res.OpenQuestions,
		Risk:          res.Risk,
		LastMessageID: lastMessageID,
		GeneratedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := u.summaries.Upsert(ctx, summary); err != nil {
		return nil, fmt.Errorf("save chat summary: %w", err)
	}

	// 9. Применение анализа: частичный успех маркируется только в логе (Q11).
	if handler := u.getAnalysisHandler(); handler != nil {
		if err := handler.HandleAnalysis(ctx, companyID, chatID, *res); err != nil {
			log.Printf("chat summary: analysis handler failed (summary saved): company_id=%d chat_id=%d err=%v", companyID, chatID, err)
		}
	}

	// 10. Сохранённое саммари.
	return &summary, nil
}

// Enqueue ставит фоновую генерацию саммари в очередь. Сигнатура совпадает с OnChatUpdated.
// Не блокирует и не возвращает ошибок; ctx вызывающего не используется.
// Повторный вызов для чата, уже стоящего в очереди, — no-op; при полном буфере задача отбрасывается.
func (u *ChatSummaryUsecase) Enqueue(_ context.Context, companyID, chatID int64) {
	if companyID <= 0 || chatID <= 0 {
		log.Printf("chat summary: enqueue dropped, invalid ids: company_id=%d chat_id=%d", companyID, chatID)
		return
	}
	if !u.aiEnabled() || u.queue == nil {
		return
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	if _, ok := u.queued[chatID]; ok {
		return
	}
	select {
	case u.queue <- summaryJob{companyID: companyID, chatID: chatID}:
		u.queued[chatID] = struct{}{}
	default:
		log.Printf("chat summary: queue is full (%d), job dropped: company_id=%d chat_id=%d", summaryQueueSize, companyID, chatID)
	}
}

// Run — единственный фоновый воркер очереди. Блокирует до ctx.Done(); оставшиеся задачи отбрасываются.
func (u *ChatSummaryUsecase) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-u.queue:
			u.mu.Lock()
			delete(u.queued, job.chatID)
			u.mu.Unlock()
			if ctx.Err() != nil {
				return
			}
			u.processJob(ctx, job)
		}
	}
}

// processJob выполняет генерацию с force=false и ровно одним повтором для retryable-ошибок AI (D6, Q7).
func (u *ChatSummaryUsecase) processJob(ctx context.Context, job summaryJob) {
	_, err := u.Generate(ctx, job.companyID, job.chatID, false)
	if err == nil {
		return
	}
	if isRetryableAIError(err) {
		timer := time.NewTimer(summaryRetryDelay(err))
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Printf("chat summary: background retry canceled: company_id=%d chat_id=%d err=%v", job.companyID, job.chatID, err)
			return
		case <-timer.C:
		}
		_, err = u.Generate(ctx, job.companyID, job.chatID, false)
		if err == nil {
			return
		}
	}
	log.Printf("chat summary: background generation failed: company_id=%d chat_id=%d err=%v", job.companyID, job.chatID, err)
}

// isRetryableAIError: повтор только для ошибок, помеченных ErrAIRetryable (2A-13, Q7):
// 429, любой 5xx провайдера и сетевые ошибки; классификацию выполняет AI-адаптер.
// Таймаут и ошибка чтения тела после 2xx (ErrUnavailable без маркера), ErrBadGateway,
// ErrValidation и ErrNotFound не повторяются.
func isRetryableAIError(err error) bool {
	if !errors.Is(err, ErrAIRetryable) {
		return false
	}
	return !errors.Is(err, domain.ErrBadGateway) &&
		!errors.Is(err, domain.ErrValidation) &&
		!errors.Is(err, domain.ErrNotFound)
}

// summaryRetryDelay возвращает Retry-After провайдера, если он есть и не больше 10s, иначе 2s.
func summaryRetryDelay(err error) time.Duration {
	var ra RetryAfterError
	if errors.As(err, &ra) {
		if d := ra.RetryAfter(); d >= 0 && d <= summaryMaxRetryAfter {
			return d
		}
	}
	return summaryRetryBackoff
}
