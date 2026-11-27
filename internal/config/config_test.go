package config

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

const testAPIKey = "sk-test-secret-value-123"

var aiEnvKeys = []string{
	"AI_API_BASE_URL",
	"AI_API_KEY",
	"AI_MODEL",
	"AI_TIMEOUT",
	"AI_MAX_MESSAGES",
	"AI_MAX_CHARS",
}

// clearAIEnv сбрасывает AI-переменные на время теста (пустое значение = не задано).
func clearAIEnv(t *testing.T) {
	t.Helper()
	for _, k := range aiEnvKeys {
		t.Setenv(k, "")
	}
}

func TestLoad_AIDefaults(t *testing.T) {
	clearAIEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := AIConfig{
		BaseURL:     "https://api.openai.com/v1",
		APIKey:      "",
		Model:       "",
		Timeout:     30 * time.Second,
		MaxMessages: 50,
		MaxChars:    12000,
	}
	if cfg.AI != want {
		t.Fatalf("AI defaults = %v, want %v", cfg.AI, want)
	}
	if cfg.AI.Enabled() {
		t.Fatal("Enabled() = true without AI_API_KEY")
	}
}

func TestLoad_AIValidValues(t *testing.T) {
	clearAIEnv(t)
	t.Setenv("AI_API_BASE_URL", " https://llm.example.com/v1 ")
	t.Setenv("AI_API_KEY", " "+testAPIKey+" ")
	t.Setenv("AI_MODEL", " gpt-test ")
	t.Setenv("AI_TIMEOUT", "1m30s")
	t.Setenv("AI_MAX_MESSAGES", "20")
	t.Setenv("AI_MAX_CHARS", " 5000 ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := AIConfig{
		BaseURL:     "https://llm.example.com/v1",
		APIKey:      testAPIKey,
		Model:       "gpt-test",
		Timeout:     90 * time.Second,
		MaxMessages: 20,
		MaxChars:    5000,
	}
	if cfg.AI != want {
		t.Fatalf("AI = %v, want %v", cfg.AI, want)
	}
	if !cfg.AI.Enabled() {
		t.Fatal("Enabled() = false with AI_API_KEY set")
	}
}

func TestLoad_AIInvalidValues(t *testing.T) {
	cases := []struct {
		key   string
		value string
	}{
		{"AI_TIMEOUT", "abc"},
		{"AI_TIMEOUT", "30"},
		{"AI_TIMEOUT", "0s"},
		{"AI_TIMEOUT", "-5s"},
		{"AI_TIMEOUT", "5m1s"},
		{"AI_TIMEOUT", "1000h"},
		{"AI_MAX_MESSAGES", "abc"},
		{"AI_MAX_MESSAGES", "0"},
		{"AI_MAX_MESSAGES", "-1"},
		{"AI_MAX_MESSAGES", "1.5"},
		{"AI_MAX_CHARS", "abc"},
		{"AI_MAX_CHARS", "0"},
		{"AI_MAX_CHARS", "-100"},
	}
	for _, tc := range cases {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			clearAIEnv(t)
			t.Setenv("AI_API_KEY", testAPIKey)
			t.Setenv(tc.key, tc.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() error = nil, want error for %s", tc.key)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.key) {
				t.Errorf("error %q does not mention %s", msg, tc.key)
			}
			if strings.Contains(msg, testAPIKey) {
				t.Errorf("error leaks API key: %q", msg)
			}
		})
	}
}

func TestLoad_AITimeoutUpperBoundInclusive(t *testing.T) {
	clearAIEnv(t)
	t.Setenv("AI_TIMEOUT", "5m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for AI_TIMEOUT=5m", err)
	}
	if cfg.AI.Timeout != MaxAITimeout {
		t.Fatalf("Timeout = %s, want %s", cfg.AI.Timeout, MaxAITimeout)
	}
}

func TestLoad_AITimeoutTooLargeMessage(t *testing.T) {
	clearAIEnv(t)
	t.Setenv("AI_TIMEOUT", "1000h")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "AI_TIMEOUT must be at most 5m") {
		t.Fatalf("Load() error = %v, want upper-bound error", err)
	}
	if strings.Contains(err.Error(), "1000h") {
		t.Errorf("error leaks value: %q", err.Error())
	}
}

func TestLoad_AIMultipleInvalidValues(t *testing.T) {
	clearAIEnv(t)
	t.Setenv("AI_TIMEOUT", "bad")
	t.Setenv("AI_MAX_MESSAGES", "0")
	t.Setenv("AI_MAX_CHARS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	for _, k := range []string{"AI_TIMEOUT", "AI_MAX_MESSAGES", "AI_MAX_CHARS"} {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error %q does not mention %s", err.Error(), k)
		}
	}
}

func TestAIConfig_Enabled(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"\t\n", false},
		{testAPIKey, true},
		{"  " + testAPIKey + "  ", true},
	}
	for _, tc := range cases {
		if got := (AIConfig{APIKey: tc.key}).Enabled(); got != tc.want {
			t.Errorf("Enabled() with key %q = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestConfig_FormatDoesNotLeakAPIKey(t *testing.T) {
	clearAIEnv(t)
	t.Setenv("AI_API_KEY", testAPIKey)
	t.Setenv("AI_MODEL", "gpt-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	outputs := map[string]string{
		"cfg %v":     fmt.Sprintf("%v", cfg),
		"cfg %+v":    fmt.Sprintf("%+v", cfg),
		"cfg %#v":    fmt.Sprintf("%#v", cfg),
		"cfg Sprint": fmt.Sprint(cfg),
		"ai %v":      fmt.Sprintf("%v", cfg.AI),
		"ai %+v":     fmt.Sprintf("%+v", cfg.AI),
		"ai %#v":     fmt.Sprintf("%#v", cfg.AI),
		"ai %s":      fmt.Sprintf("%s", cfg.AI),
		"ai String":  cfg.AI.String(),
		"ai pointer": fmt.Sprintf("%+v", &cfg.AI),
	}
	for name, out := range outputs {
		if strings.Contains(out, testAPIKey) {
			t.Errorf("%s leaks API key: %q", name, out)
		}
	}
	if !strings.Contains(outputs["cfg %+v"], "[REDACTED]") {
		t.Errorf("cfg %%+v = %q, want masked key marker", outputs["cfg %+v"])
	}
	if !strings.Contains(outputs["cfg %+v"], "gpt-test") {
		t.Errorf("cfg %%+v = %q, want model to be visible", outputs["cfg %+v"])
	}
}

func TestMust_AlertCheckInterval(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		clearAIEnv(t)
		t.Setenv("NO_REPLY_CHECK_INTERVAL", "")
		t.Setenv("ALERT_CHECK_INTERVAL", "")
		cfg := Must()
		if cfg.Alerts.CheckInterval != DefaultAlertCheckInterval {
			t.Fatalf("expected %s, got %s", DefaultAlertCheckInterval, cfg.Alerts.CheckInterval)
		}
	})

	t.Run("prefers NO_REPLY_CHECK_INTERVAL with trim", func(t *testing.T) {
		clearAIEnv(t)
		t.Setenv("NO_REPLY_CHECK_INTERVAL", " 45s ")
		t.Setenv("ALERT_CHECK_INTERVAL", "15s")
		cfg := Must()
		if cfg.Alerts.CheckInterval != 45*time.Second {
			t.Fatalf("expected 45s, got %s", cfg.Alerts.CheckInterval)
		}
	})

	t.Run("custom positive duration via ALERT_CHECK_INTERVAL fallback", func(t *testing.T) {
		clearAIEnv(t)
		t.Setenv("NO_REPLY_CHECK_INTERVAL", "")
		t.Setenv("ALERT_CHECK_INTERVAL", " 15s ")
		cfg := Must()
		if cfg.Alerts.CheckInterval != 15*time.Second {
			t.Fatalf("expected 15s, got %s", cfg.Alerts.CheckInterval)
		}
	})

	t.Run("invalid or non-positive falls back to default", func(t *testing.T) {
		for _, val := range []string{"invalid", "0s", "-5s"} {
			clearAIEnv(t)
			t.Setenv("NO_REPLY_CHECK_INTERVAL", "")
			t.Setenv("ALERT_CHECK_INTERVAL", val)
			cfg := Must()
			if cfg.Alerts.CheckInterval != DefaultAlertCheckInterval {
				t.Fatalf("for %q expected default %s, got %s", val, DefaultAlertCheckInterval, cfg.Alerts.CheckInterval)
			}
		}
	})
}

func TestConfig_FormatDoesNotLeakDBOrTelegramSecrets(t *testing.T) {
	clearAIEnv(t)
	t.Setenv("DB_PASSWORD", "super-secret-db-pass")
	t.Setenv("TELEGRAM_API_HASH", "super-secret-tg-hash")
	t.Setenv("TELEGRAM_PROXY", "http://proxyuser:proxypass@127.0.0.1:8080")
	t.Setenv("USE_PROXY", "true")
	t.Setenv("TELEGRAM_SESSION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, out := range []string{
		fmt.Sprintf("%v", cfg),
		fmt.Sprintf("%+v", cfg),
		fmt.Sprintf("%#v", cfg),
		cfg.DB.String(),
		cfg.Telegram.String(),
	} {
		for _, secret := range []string{"super-secret-db-pass", "super-secret-tg-hash", "proxypass"} {
			if strings.Contains(out, secret) {
				t.Errorf("formatted config leaks %q: %s", secret, out)
			}
		}
	}
}
