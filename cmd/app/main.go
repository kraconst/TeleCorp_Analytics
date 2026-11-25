package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kraconst/GO_server/internal/ai"
	"github.com/kraconst/GO_server/internal/config"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/storage/postgres"
	"github.com/kraconst/GO_server/internal/usecase"
)

const (
	// workerStopTimeout — сколько ждать фоновые воркеры после отмены их контекста:
	// отмена сразу прерывает AI-запрос, поэтому фиксированных 5s достаточно.
	workerStopTimeout = 5 * time.Second
	// requestReserve — запас сверх AI_TIMEOUT для синхронного generate (RFC 02, Q4).
	requestReserve = 5 * time.Second
)

func main() {
	cfg := config.Must()

	summarizer, err := newSummarizer(cfg.AI)
	if err != nil {
		log.Fatalf("AI client configuration failed: %v", err)
	}

	db, err := postgres.New(cfg.DB.DSN())
	if err != nil {
		log.Fatalf("PostgreSQL connection failed: %v (run 'docker compose up -d' to start database)", err)
	}
	defer db.Close()
	log.Println("Connected to PostgreSQL successfully")

	// Run pending migrations automatically with advisory lock
	if err := postgres.RunMigrations(db, "migrations"); err != nil {
		log.Fatalf("Database migrations failed: %v", err)
	}
	log.Println("Database migrations applied successfully")

	accountRepo := postgres.NewAccountStorage(db)
	employeeRepo := postgres.NewEmployeeStorage(db)
	messageRepo := postgres.NewMessageStorage(db)
	contactRepo := postgres.NewContactStorage(db)
	chatRepo := postgres.NewChatStorage(db)
	summaryRepo := postgres.NewChatSummaryStorage(db)
	accountDailyStatsRepo := postgres.NewAccountDailyStatsStorage(db)
	alertRepo := postgres.NewAlertStorage(db)
	companyRepo := postgres.NewCompanyStorage(db)
	userRepo := postgres.NewUserStorage(db)

	accountUsecase := usecase.NewAccountUsecase(accountRepo, employeeRepo)
	employeeUsecase := usecase.NewEmployeeUsecase(employeeRepo)
	messageUsecase := usecase.NewMessageUsecase(messageRepo, chatRepo, accountRepo)
	contactUsecase := usecase.NewContactUsecase(contactRepo)
	chatUsecase := usecase.NewChatUsecase(chatRepo, contactRepo, summaryRepo, accountRepo)
	accountDailyStatsUsecase := usecase.NewAccountDailyStatsUsecase(accountDailyStatsRepo, accountRepo)
	alertUsecase := usecase.NewAlertUsecase(alertRepo, chatRepo, accountRepo)
	companyUsecase := usecase.NewCompanyUsecase(companyRepo)
	// ChatAnalysisHandler = nil до трека 2B (автостатусы/алерты).
	// Когда Группа 1 добавит SyncUsecase, передать summaryUsecase.Enqueue как OnChatUpdated (RFC 02, D4).
	summaryUsecase := usecase.NewChatSummaryUsecaseWithAI(
		summaryRepo, chatRepo, accountRepo, messageRepo, summarizer, nil, cfg.AI.MaxMessages,
	)
	userUsecase := usecase.NewUserUsecase(userRepo, employeeRepo)

	h := handler.New(
		accountUsecase,
		employeeUsecase,
		messageUsecase,
		contactUsecase,
		chatUsecase,
		accountDailyStatsUsecase,
		alertUsecase,
		companyUsecase,
		summaryUsecase,
		userUsecase,
	)

	// requestBudget — WriteTimeout и время graceful shutdown: синхронный generate
	// успевает завершиться (AI_TIMEOUT ≤ 5m гарантирует config, переполнения нет).
	requestBudget := cfg.AI.Timeout + requestReserve

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      h.InitRoutes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: requestBudget,
		IdleTimeout:  60 * time.Second,
	}

	sigCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	// Контекст воркеров отделён от сигнального: воркеры отменяются только после srv.Shutdown.
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()

	// Воркер запускается всегда: без AI Enqueue — no-op, Run просто ждёт отмены.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		summaryUsecase.Run(workerCtx)
	}()

	go func() {
		log.Printf("Server listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server listen failed: %v", err)
		}
	}()

	<-sigCtx.Done()
	stopSignals() // повторный сигнал завершает процесс без ожидания
	log.Println("Shutting down server on signal...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), requestBudget)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Отмена прерывает in-flight AI-запрос воркера (запрос привязан к ctx).
	cancelWorkers()
	workersDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(workersDone)
	}()
	select {
	case <-workersDone:
		log.Println("Background workers stopped")
	case <-time.After(workerStopTimeout):
		log.Printf("Background workers did not stop within %s", workerStopTimeout)
	}
	log.Println("Server gracefully stopped")
}

// newSummarizer собирает AI-клиент. Без ключа или модели возвращает литеральный nil
// (AI отключён, generate → 503). Ни ключ, ни его фрагменты не логируются.
func newSummarizer(cfg config.AIConfig) (usecase.ChatSummarizer, error) {
	if !cfg.Enabled() {
		log.Println("AI отключён: AI_API_KEY не задан")
		return nil, nil
	}
	if strings.TrimSpace(cfg.Model) == "" {
		log.Println("AI отключён: не задан AI_MODEL")
		return nil, nil
	}
	client, err := ai.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.Timeout, cfg.MaxChars)
	if err != nil {
		return nil, err
	}
	log.Printf("AI включён: model=%s, base=%s", cfg.Model, baseHost(cfg.BaseURL))
	return client, nil
}

// baseHost возвращает только host base URL (без пути); URL уже проверен ai.NewClient.
func baseHost(raw string) string {
	if raw == "" {
		raw = ai.DefaultBaseURL
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "invalid"
	}
	return u.Host
}
