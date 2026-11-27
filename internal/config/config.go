package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// Значения AI-конфигурации по умолчанию (RFC 02, D10).
const (
	DefaultAIBaseURL = "https://api.openai.com/v1"
	DefaultAITimeout = 30 * time.Second
	// MaxAITimeout — верхняя граница AI_TIMEOUT; исключает переполнение AI_TIMEOUT + 5s.
	MaxAITimeout         = 5 * time.Minute
	DefaultAIMaxMessages = 50
	DefaultAIMaxChars    = 12000
)

const DefaultAlertCheckInterval = time.Minute

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

// AIConfig — настройки OpenAI-совместимого провайдера (RFC 02, D10).
// Пустой APIKey означает, что AI отключён. APIKey не выводится через fmt (см. String/GoString).
type AIConfig struct {
	BaseURL     string
	APIKey      string
	Model       string
	Timeout     time.Duration
	MaxMessages int
	MaxChars    int
}

type AlertConfig struct {
	CheckInterval time.Duration
}

// Enabled сообщает, задан ли API-ключ.
func (c AIConfig) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != ""
}

// String возвращает представление без значения API-ключа.
func (c AIConfig) String() string {
	return fmt.Sprintf("{BaseURL:%s APIKey:%s Model:%s Timeout:%s MaxMessages:%d MaxChars:%d}",
		c.BaseURL, c.maskedKey(), c.Model, c.Timeout, c.MaxMessages, c.MaxChars)
}

// GoString возвращает представление для %#v без значения API-ключа.
func (c AIConfig) GoString() string {
	return fmt.Sprintf("config.AIConfig{BaseURL:%q, APIKey:%q, Model:%q, Timeout:%d, MaxMessages:%d, MaxChars:%d}",
		c.BaseURL, c.maskedKey(), c.Model, int64(c.Timeout), c.MaxMessages, c.MaxChars)
}

func (c AIConfig) maskedKey() string {
	if c.Enabled() {
		return "[REDACTED]"
	}
	return "[EMPTY]"
}

type Config struct {
	Addr     string
	DB       DBConfig
	Telegram TelegramConfig
	AI       AIConfig
	Alerts   AlertConfig
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

// Load читает конфигурацию из окружения. Ошибка содержит имена некорректных переменных,
// но не их значения.
func Load() (Config, error) {
	var errs []error

	timeout, err := positiveDurationEnv("AI_TIMEOUT", DefaultAITimeout)
	if err != nil {
		errs = append(errs, err)
	} else if timeout > MaxAITimeout {
		errs = append(errs, errors.New("config: AI_TIMEOUT must be at most 5m"))
	}
	maxMessages, err := positiveIntEnv("AI_MAX_MESSAGES", DefaultAIMaxMessages)
	if err != nil {
		errs = append(errs, err)
	}
	maxChars, err := positiveIntEnv("AI_MAX_CHARS", DefaultAIMaxChars)
	if err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}

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
		AI: AIConfig{
			BaseURL:     strings.TrimSpace(getEnv("AI_API_BASE_URL", DefaultAIBaseURL)),
			APIKey:      strings.TrimSpace(os.Getenv("AI_API_KEY")),
			Model:       strings.TrimSpace(os.Getenv("AI_MODEL")),
			Timeout:     timeout,
			MaxMessages: maxMessages,
			MaxChars:    maxChars,
		},
		Alerts: AlertConfig{
			CheckInterval: getDurationEnv("ALERT_CHECK_INTERVAL", DefaultAlertCheckInterval),
		},
	}, nil
}

// Must загружает конфигурацию и завершает процесс при ошибке.
func Must() Config {
	cfg, err := Load()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	return cfg
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

// positiveDurationEnv: пусто → fallback; иначе time.ParseDuration и > 0.
func positiveDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("config: %s must be a positive duration (e.g. 30s)", key)
	}
	return d, nil
}

// positiveIntEnv: пусто → fallback; иначе целое > 0.
func positiveIntEnv(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("config: %s must be a positive integer", key)
	}
	return n, nil
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	d, err := time.ParseDuration(val)
	if err != nil || d <= 0 {
		log.Printf("Invalid %s=%q, using default %s", key, val, fallback)
		return fallback
	}
	return d
}
