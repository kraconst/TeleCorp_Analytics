package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/kraconst/GO_server/internal/config"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/storage/postgres"
	"github.com/kraconst/GO_server/internal/telegram"
	"github.com/kraconst/GO_server/internal/usecase"
)

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
	summaryUsecase := usecase.NewChatSummaryUsecase(summaryRepo, chatRepo, accountRepo)
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
	appCtx, cancelApp := context.WithCancel(ctx)
	defer cancelApp()
	var workers sync.WaitGroup
	client, err := telegram.NewSyncClient(cfg.Telegram, accountRepo)
	if err != nil {
		log.Printf("Telegram sync disabled: %v", err)
	} else {
		syncer := usecase.NewSyncUsecase(postgres.NewTelegramSyncStorage(db), client, usecase.SyncOptions{})
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

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      h.InitRoutes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext:  func(net.Listener) context.Context { return appCtx },
	}

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
	cancelApp()
	log.Print("Shutting down server and Telegram worker")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdownErr := srv.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, srv.Close())
	}
	workers.Wait()
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
