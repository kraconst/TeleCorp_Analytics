package config_test

import (
	"testing"
	"time"

	"github.com/kraconst/GO_server/internal/config"
)

func TestMust_AlertCheckInterval(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		t.Setenv("ALERT_CHECK_INTERVAL", "")
		cfg := config.Must()
		if cfg.Alerts.CheckInterval != config.DefaultAlertCheckInterval {
			t.Fatalf("expected %s, got %s", config.DefaultAlertCheckInterval, cfg.Alerts.CheckInterval)
		}
	})

	t.Run("custom positive duration", func(t *testing.T) {
		t.Setenv("ALERT_CHECK_INTERVAL", "15s")
		cfg := config.Must()
		if cfg.Alerts.CheckInterval != 15*time.Second {
			t.Fatalf("expected 15s, got %s", cfg.Alerts.CheckInterval)
		}
	})

	t.Run("invalid or non-positive falls back to default", func(t *testing.T) {
		for _, val := range []string{"invalid", "0s", "-5s"} {
			t.Setenv("ALERT_CHECK_INTERVAL", val)
			cfg := config.Must()
			if cfg.Alerts.CheckInterval != config.DefaultAlertCheckInterval {
				t.Fatalf("for %q expected default %s, got %s", val, config.DefaultAlertCheckInterval, cfg.Alerts.CheckInterval)
			}
		}
	})
}
