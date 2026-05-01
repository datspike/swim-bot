// Package config отвечает за загрузку конфигурации из переменных окружения.
package config

import (
	"errors"
	"os"
	"strconv"
)

// Config содержит конфигурацию приложения.
type Config struct {
	// TelegramToken — токен бота от @BotFather.
	TelegramToken string
	// WebhookURL — URL для получения updates от Telegram.
	WebhookURL string
	// WebhookSecret — секрет для проверки X-Telegram-Bot-Api-Secret-Token.
	WebhookSecret string
	// Port — порт HTTP сервера.
	Port int
	// LogLevel — уровень логирования (debug, info, warn, error).
	LogLevel string
	// DBPath — путь к SQLite базе данных.
	DBPath string
	// MaxMessageAgeSec — порог пропуска старых сообщений в секундах (FR-011, по умолчанию 30).
	MaxMessageAgeSec int
	// AutoDeleteSpamMessageSec — автоудаление спам-сообщения через N секунд (0 = выключено).
	AutoDeleteSpamMessageSec int
}

// ErrMissingEnv возвращается когда обязательная переменная окружения не задана.
var ErrMissingEnv = errors.New("отсутствует обязательная переменная окружения")

// Load загружает конфигурацию из переменных окружения.
// Возвращает ошибку если обязательные переменные не заданы.
func Load() (*Config, error) {
	cfg := &Config{}

	// обязательные переменные
	cfg.TelegramToken = os.Getenv("TELEGRAM_TOKEN")
	if cfg.TelegramToken == "" {
		return nil, errors.Join(ErrMissingEnv, errors.New("TELEGRAM_TOKEN"))
	}

	cfg.WebhookURL = os.Getenv("WEBHOOK_URL")
	if cfg.WebhookURL == "" {
		return nil, errors.Join(ErrMissingEnv, errors.New("WEBHOOK_URL"))
	}

	// необязательные переменные с дефолтами
	cfg.WebhookSecret = os.Getenv("WEBHOOK_SECRET")

	cfg.DBPath = os.Getenv("DB_PATH")
	if cfg.DBPath == "" {
		cfg.DBPath = "data.db"
	}
	portStr := os.Getenv("PORT")
	if portStr == "" {
		cfg.Port = 8080
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, errors.New("PORT должен быть числом")
		}
		cfg.Port = port
	}

	cfg.LogLevel = os.Getenv("LOG_LEVEL")
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	maxAgeStr := os.Getenv("MAX_MESSAGE_AGE_SEC")
	if maxAgeStr == "" {
		cfg.MaxMessageAgeSec = 30
	} else {
		age, err := strconv.Atoi(maxAgeStr)
		if err != nil {
			return nil, errors.New("MAX_MESSAGE_AGE_SEC должен быть числом")
		}
		cfg.MaxMessageAgeSec = age
	}

	autoDeleteSpamMsgStr := os.Getenv("AUTO_DELETE_SPAM_MESSAGE_SEC")
	if autoDeleteSpamMsgStr == "" {
		cfg.AutoDeleteSpamMessageSec = 0
	} else {
		ttl, err := strconv.Atoi(autoDeleteSpamMsgStr)
		if err != nil {
			return nil, errors.New("AUTO_DELETE_SPAM_MESSAGE_SEC должен быть числом")
		}
		if ttl < 0 {
			return nil, errors.New("AUTO_DELETE_SPAM_MESSAGE_SEC не может быть отрицательным")
		}
		cfg.AutoDeleteSpamMessageSec = ttl
	}

	return cfg, nil
}
