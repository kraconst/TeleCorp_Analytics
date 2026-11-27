package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kraconst/GO_server/internal/ai"
	"github.com/kraconst/GO_server/internal/config"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/storage/postgres"
	"github.com/kraconst/GO_server/internal/telegram"
	"github.com/kraconst/GO_server/internal/usecase"
)

const (
	// workerStopTimeout — сколько ждать фоновые воркеры после отмены их контекста:
	// отмена сразу прерывает AI-запрос, поэтому фиксированных 5s достаточно.
	workerStopTimeout = 5 * time.Second
	// requestReserve — запас сверх AI_TIMEOUT для синхронного generate (RFC 02, Q4).
	requestReserve = 5 * time.Second
)

type noReplyAlertChecker interface {
	CheckNoReplyAlerts(ctx context.Context) (int, error)
}

func runNoReplyAlertWorker(ctx context.Context, checker noReplyAlertChecker, interval time.Duration) {
	if checker == nil {
		return
	}
	if interval <= 0 {
		interval = config.DefaultAlertCheckInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			created, err := checker.CheckNoReplyAlerts(checkCtx)
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("No-reply alert check error: %v", err)
			} else if created > 0 {
				log.Printf("Created %d no-reply alert(s)", created)
			}
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cfg := config.Must()
	var enrollment telegramImportArgs
	if len(args) > 0 {
		var err error
		enrollment, err = parseTelegramImportArgs(args)
		if err != nil {
			return err
		}
		if !cfg.Telegram.Configured() {
			return usecase.ErrTelegramUnavailable
		}
	}

	db, err := postgres.New(cfg.DB.DSN())
	if err != nil {
		return fmt.Errorf("PostgreSQL connection failed: %w (run 'docker compose up -d' to start database)", err)
	}
	defer db.Close()
	log.Println("Connected to PostgreSQL successfully")
	accountRepo := postgres.NewAccountStorage(db)
	if len(args) > 0 {
		return runTelegramImport(ctx, cfg.Telegram, accountRepo, enrollment)
	}

	summarizer, err := newSummarizer(cfg.AI)
	if err != nil {
		return fmt.Errorf("AI client configuration failed: %w", err)
	}

	// Run pending migrations automatically with advisory lock
	if err := postgres.RunMigrations(db, "migrations"); err != nil {
		return fmt.Errorf("database migrations failed: %w", err)
	}
	log.Println("Database migrations applied successfully")

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
	summaryUsecase := usecase.NewChatSummaryUsecaseWithAI(
		summaryRepo, chatRepo, accountRepo, messageRepo, summarizer,
		usecase.AIAnalysisHandler(alertUsecase.ApplyAIAnalysis), cfg.AI.MaxMessages,
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
	// appCtx отделён от сигнального ctx, чтобы srv.Shutdown успел завершить in-flight HTTP-запросы
	// в пределах requestBudget, и только потом cancelApp() останавливал фоновые воркеры.
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()
	var workers sync.WaitGroup
	client, err := telegram.NewSyncClient(cfg.Telegram, accountRepo)
	if err != nil {
		log.Printf("Telegram sync disabled: %v", err)
	} else {
		syncer := usecase.NewSyncUsecase(postgres.NewTelegramSyncStorage(db), client, usecase.SyncOptions{
			OnChatUpdated: summaryUsecase.Enqueue,
		})
		h.SetAccountSync(syncer)
		workers.Go(func() { syncer.RunNotifications(appCtx) })
		if cfg.Telegram.SyncEnabled {
			workers.Go(func() {
				syncer.RunWorker(appCtx, cfg.Telegram.SyncInterval, reportTelegramSync)
			})
		} else {
			log.Print("Telegram scheduled sync disabled; manual sync is available")
		}
	}

	// requestBudget — WriteTimeout и время graceful shutdown: синхронный generate
	// успевает завершиться (AI_TIMEOUT ≤ 5m гарантирует config, переполнения нет).
	requestBudget := cfg.AI.Timeout + requestReserve

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      h.InitRoutes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: requestBudget,
		IdleTimeout:  60 * time.Second,
		BaseContext:  func(net.Listener) context.Context { return appCtx },
	}

	// Воркер AI-саммаризации запускается всегда: без AI Enqueue — no-op, Run просто ждёт отмены.
	workers.Go(func() { summaryUsecase.Run(appCtx) })
	workers.Go(func() {
		log.Printf("No-reply alert worker started (interval=%s)", cfg.Alerts.CheckInterval)
		runNoReplyAlertWorker(appCtx, alertUsecase, cfg.Alerts.CheckInterval)
		log.Println("No-reply alert worker stopped")
	})

	serveDone := make(chan error, 1)
	go func() {
		log.Printf("Server listening on %s", cfg.Addr)
		serveDone <- srv.ListenAndServe()
	}()

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-serveDone:
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
	}
	log.Print("Shutting down server and background workers")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), requestBudget)
	defer cancel()

	shutdownErr := srv.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, srv.Close())
	}
	cancelApp()
	workersDone := make(chan struct{})
	go func() {
		workers.Wait()
		close(workersDone)
	}()
	select {
	case <-workersDone:
		log.Println("Background workers stopped")
	case <-time.After(workerStopTimeout):
		log.Printf("Background workers did not stop within %s", workerStopTimeout)
	}
	return errors.Join(serveErr, shutdownErr)
}

func reportTelegramSync(account usecase.SyncAccountRef, result usecase.SyncResult, err error) {
	if err != nil {
		if account.AccountID == 0 {
			log.Printf("Telegram sync could not list accounts: %v", err)
			return
		}
		log.Printf("Telegram sync failed for company %d account %d after %d chats and %d messages: %v",
			account.CompanyID, account.AccountID, result.ChatsProcessed, result.NewMessages, err)
	}
	if result.NotificationsDropped > 0 {
		log.Printf("Telegram sync dropped %d chat notifications for company %d account %d",
			result.NotificationsDropped, account.CompanyID, account.AccountID)
	}
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
