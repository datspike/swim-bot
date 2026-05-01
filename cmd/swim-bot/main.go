// Package main — точка входа для swim-bot.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/datspike/swim-bot/internal/bot"
	"github.com/datspike/swim-bot/internal/config"
	"github.com/datspike/swim-bot/internal/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ошибка: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// загрузка конфигурации
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("не удалось загрузить конфигурацию: %w", err)
	}

	// настройка логгера
	logger := setupLogger(cfg.LogLevel)
	logger.Info("запуск swim-bot", "log_level", cfg.LogLevel)

	// подключение к БД
	store, err := storage.NewStorage(cfg.DBPath, logger)
	if err != nil {
		return fmt.Errorf("не удалось подключиться к БД: %w", err)
	}
	defer store.Close()

	// миграции
	if migrateErr := storage.Migrate(store.DB(), logger); migrateErr != nil {
		return fmt.Errorf("не удалось выполнить миграции: %w", migrateErr)
	}

	// активация чатов с tracked_bot, ожидавших стикер
	if activateErr := store.ActivateConfiguredChats(); activateErr != nil {
		return fmt.Errorf("не удалось активировать настроенные чаты: %w", activateErr)
	}

	// создание бота
	b, err := bot.New(bot.Config{
		Token:         cfg.TelegramToken,
		WebhookURL:    cfg.WebhookURL,
		WebhookSecret: cfg.WebhookSecret,
		Port:          cfg.Port,
		MaxMessageAge: time.Duration(cfg.MaxMessageAgeSec) * time.Second,
		SpamDeleteTTL: time.Duration(cfg.AutoDeleteSpamMessageSec) * time.Second,
		Storage:       store,
		Logger:        logger,
	})
	if err != nil {
		return fmt.Errorf("не удалось создать бота: %w", err)
	}

	// graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// запуск бота в горутине
	go b.Start()

	// ожидание сигнала завершения
	<-ctx.Done()
	logger.Info("получен сигнал завершения, останавливаем бота...")

	// graceful shutdown с таймаутом
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	done := make(chan struct{})
	go func() {
		b.Stop()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("бот остановлен")
	case <-shutdownCtx.Done():
		logger.Warn("таймаут при остановке бота")
	}

	return nil
}

// setupLogger создаёт логгер с указанным уровнем.
func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})

	return slog.New(handler)
}
