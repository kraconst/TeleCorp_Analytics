package config

import (
	"fmt"
	"log"
	"os"
	"time"
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

type AlertConfig struct {
	CheckInterval time.Duration
}

type Config struct {
	Addr   string
	DB     DBConfig
	Alerts AlertConfig
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
		Alerts: AlertConfig{
			CheckInterval: getDurationEnv("ALERT_CHECK_INTERVAL", DefaultAlertCheckInterval),
		},
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
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
