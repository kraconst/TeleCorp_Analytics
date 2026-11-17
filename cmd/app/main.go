package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kraconst/GO_server/internal/config"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/storage/postgres"
	"github.com/kraconst/GO_server/internal/usecase"
)

func main() {
	cfg := config.Must()

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

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      h.InitRoutes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Server listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server listen failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("Shutting down server on signal: %v...", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server gracefully stopped")
}
