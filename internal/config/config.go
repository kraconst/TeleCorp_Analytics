package config

import (
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

func (c DBConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode)
}

type Config struct {
	Addr     string
	DB       DBConfig
	Telegram TelegramConfig
}

type TelegramConfig struct {
	APIID          int
	APIHash        string
	Proxy          string
	SessionKey     []byte
	SyncEnabled    bool
	SyncInterval   time.Duration
	SyncBatchLimit int
}

// Configured reports whether a manual sync can use the server credentials.
func (c TelegramConfig) Configured() bool {
	return c.APIID > 0 && c.APIID <= math.MaxInt32 && strings.TrimSpace(c.APIHash) != "" && len(c.SessionKey) == 32
}

func Must() Config {
	return Config{
		Addr: getEnv("HTTP_ADDR", ":8080"),
		DB: DBConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "telecorp"),
			Password: getEnv("DB_PASSWORD", "telecorp_secret"),
			Name:     getEnv("DB_NAME", "telecorp_db"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Telegram: loadTelegram(),
	}
}

func loadTelegram() TelegramConfig {
	apiID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("TELEGRAM_API_ID")), 10, 32)
	if err != nil || apiID <= 0 {
		apiID = 0
	}
	interval := time.Minute
	if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv("TELEGRAM_SYNC_INTERVAL"))); err == nil && value > 0 {
		interval = value
	}
	batchLimit := 100
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("TELEGRAM_SYNC_BATCH_LIMIT"))); err == nil && value >= 1 && value <= 100 {
		batchLimit = value
	}

	proxy := strings.TrimSpace(os.Getenv("TELEGRAM_PROXY"))
	useProxy, useProxyErr := strconv.ParseBool(strings.TrimSpace(os.Getenv("USE_PROXY")))
	noProxy, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("TELEGRAM_NO_PROXY")))
	if noProxy || (useProxyErr == nil && !useProxy) {
		proxy = ""
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("TELEGRAM_SESSION_KEY")))
	if err != nil || len(key) != 32 {
		key = nil
	}
	cfg := TelegramConfig{
		APIID:          int(apiID),
		APIHash:        strings.TrimSpace(os.Getenv("TELEGRAM_API_HASH")),
		Proxy:          proxy,
		SessionKey:     key,
		SyncInterval:   interval,
		SyncBatchLimit: batchLimit,
	}
	enabled, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("TELEGRAM_SYNC_ENABLED")))
	cfg.SyncEnabled = enabled && cfg.Configured()
	return cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
