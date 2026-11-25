package config

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestTelegramConfigDefaultsAndDisabledCredentials(t *testing.T) {
	t.Setenv("TELEGRAM_API_ID", "2147483648")
	t.Setenv("TELEGRAM_API_HASH", "test-hash")
	t.Setenv("TELEGRAM_SESSION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("TELEGRAM_SYNC_ENABLED", "true")
	t.Setenv("TELEGRAM_SYNC_INTERVAL", "-2s")
	t.Setenv("TELEGRAM_SYNC_BATCH_LIMIT", "101")
	cfg := Must().Telegram
	if cfg.Configured() || cfg.SyncEnabled || cfg.APIID != 0 {
		t.Fatal("an API ID outside the Telegram integer range must disable sync")
	}
	if cfg.SyncInterval != time.Minute || cfg.SyncBatchLimit != 100 {
		t.Fatalf("unexpected defaults: %v, %d", cfg.SyncInterval, cfg.SyncBatchLimit)
	}

	t.Setenv("TELEGRAM_API_ID", "123")
	t.Setenv("TELEGRAM_SESSION_KEY", "malformed")
	if cfg := Must().Telegram; cfg.Configured() || cfg.SyncEnabled {
		t.Fatal("an invalid session key must disable sync")
	}
}

func TestTelegramConfigProxyAndWorkerControls(t *testing.T) {
	t.Setenv("TELEGRAM_API_ID", "123")
	t.Setenv("TELEGRAM_API_HASH", " test-hash ")
	t.Setenv("TELEGRAM_SESSION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("TELEGRAM_SYNC_INTERVAL", "30s")
	t.Setenv("TELEGRAM_SYNC_BATCH_LIMIT", "25")
	t.Setenv("TELEGRAM_PROXY", " socks5://proxy.example:1080 ")

	for _, tc := range []struct {
		useProxy, noProxy, syncEnabled string
		proxy, worker                  bool
	}{
		{"", "", "", true, false},
		{"0", "false", "true", false, true},
		{"true", "true", "1", false, true},
		{"true", "false", "true", true, true},
	} {
		t.Setenv("USE_PROXY", tc.useProxy)
		t.Setenv("TELEGRAM_NO_PROXY", tc.noProxy)
		t.Setenv("TELEGRAM_SYNC_ENABLED", tc.syncEnabled)
		cfg := Must().Telegram
		if !cfg.Configured() || cfg.SyncEnabled != tc.worker || (cfg.Proxy != "") != tc.proxy {
			t.Errorf("unexpected readiness, worker or proxy setting for %+v", tc)
		}
		if cfg.APIID != 123 || cfg.APIHash != "test-hash" || cfg.SyncInterval != 30*time.Second || cfg.SyncBatchLimit != 25 {
			t.Error("configured Telegram options were not parsed")
		}
	}
}
