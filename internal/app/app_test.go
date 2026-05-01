package app

import (
	"log/slog"
	"testing"
	"time"

	"github.com/datspike/swim-bot/internal/config"
)

type fakeStopper struct {
	started chan struct{}
	stopped chan struct{}
	block   chan struct{}
}

func (f *fakeStopper) Stop() {
	if f.started != nil {
		close(f.started)
	}
	if f.block != nil {
		<-f.block
	}
	close(f.stopped)
}

func TestBuildBotConfigMapsRuntimeConfig(t *testing.T) {
	// проверка переноса runtime-конфигурации в конфигурацию бота
	cfg := &config.Config{
		TelegramToken:    "token",
		WebhookURL:       "https://example.test/webhook",
		WebhookSecret:    "secret",
		Port:             9090,
		MaxMessageAgeSec: 45,
	}
	logger := slog.Default()

	got := buildBotConfig(cfg, nil, logger)

	if got.Token != cfg.TelegramToken {
		t.Fatalf("Token = %q, want %q", got.Token, cfg.TelegramToken)
	}
	if got.WebhookURL != cfg.WebhookURL {
		t.Fatalf("WebhookURL = %q, want %q", got.WebhookURL, cfg.WebhookURL)
	}
	if got.WebhookSecret != cfg.WebhookSecret {
		t.Fatalf("WebhookSecret = %q, want %q", got.WebhookSecret, cfg.WebhookSecret)
	}
	if got.Port != cfg.Port {
		t.Fatalf("Port = %d, want %d", got.Port, cfg.Port)
	}
	if got.MaxMessageAge != 45*time.Second {
		t.Fatalf("MaxMessageAge = %s, want 45s", got.MaxMessageAge)
	}
	if got.Logger != logger {
		t.Fatal("Logger не совпадает с переданным")
	}
}

func TestStopBotStopsRunner(t *testing.T) {
	// проверка успешной остановки бота без ожидания таймаута
	stopper := &fakeStopper{stopped: make(chan struct{})}

	stopBot(slog.Default(), stopper)

	select {
	case <-stopper.stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop не был вызван")
	}
}

func TestStopBotReturnsOnTimeout(t *testing.T) {
	// проверка выхода по таймауту при зависшей остановке бота
	stopper := &fakeStopper{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
		block:   make(chan struct{}),
	}

	startedAt := time.Now()
	stopBotWithTimeout(slog.Default(), stopper, 10*time.Millisecond)
	elapsed := time.Since(startedAt)

	select {
	case <-stopper.started:
	default:
		t.Fatal("Stop не был вызван")
	}
	if elapsed > time.Second {
		t.Fatalf("stopBotWithTimeout вернулся через %s, ожидался быстрый возврат", elapsed)
	}

	close(stopper.block)
	select {
	case <-stopper.stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop не завершился после разблокировки")
	}
}
