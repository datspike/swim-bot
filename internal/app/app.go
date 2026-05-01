// Package app управляет запуском и остановкой swim-bot.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/datspike/swim-bot/internal/bot"
	"github.com/datspike/swim-bot/internal/config"
	"github.com/datspike/swim-bot/internal/storage"
)

const shutdownTimeout = 15 * time.Second

// Run запускает приложение и блокируется до отмены контекста.
func Run(ctx context.Context) error {
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
	telegramBot, err := bot.New(buildBotConfig(cfg, store, logger))
	if err != nil {
		return fmt.Errorf("не удалось создать бота: %w", err)
	}

	// запуск бота в горутине
	go telegramBot.Start()

	// ожидание сигнала завершения
	<-ctx.Done()
	logger.Info("получен сигнал завершения, останавливаем бота...")

	stopBot(logger, telegramBot)
	return nil
}

func buildBotConfig(cfg *config.Config, store *storage.Storage, logger *slog.Logger) bot.Config {
	return bot.Config{
		Token:         cfg.TelegramToken,
		WebhookURL:    cfg.WebhookURL,
		WebhookSecret: cfg.WebhookSecret,
		Port:          cfg.Port,
		MaxMessageAge: time.Duration(cfg.MaxMessageAgeSec) * time.Second,
		Storage:       store,
		Logger:        logger,
	}
}

func stopBot(logger *slog.Logger, telegramBot interface{ Stop() }) {
	stopBotWithTimeout(logger, telegramBot, shutdownTimeout)
}

func stopBotWithTimeout(logger *slog.Logger, telegramBot interface{ Stop() }, timeout time.Duration) {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), timeout)
	defer shutdownCancel()

	done := make(chan struct{})
	go func() {
		telegramBot.Stop()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("бот остановлен")
	case <-shutdownCtx.Done():
		logger.Warn("таймаут при остановке бота")
	}
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
