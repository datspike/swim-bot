package bot

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/datspike/swim-bot/internal/storage"

	tele "gopkg.in/telebot.v3"
)

// Bot содержит инициализированного Telegram бота.
type Bot struct {
	bot     *tele.Bot
	storage *storage.Storage
	logger  *slog.Logger
}

// Config содержит параметры для создания бота.
type Config struct {
	Token         string
	WebhookURL    string
	WebhookSecret string
	Port          int
	Storage       *storage.Storage
	Logger        *slog.Logger
}

// New создаёт и настраивает Telegram бота с webhook poller.
func New(cfg Config) (*Bot, error) {
	// настройка webhook
	webhook := &tele.Webhook{
		Listen: fmt.Sprintf(":%d", cfg.Port),
		Endpoint: &tele.WebhookEndpoint{
			PublicURL: cfg.WebhookURL,
		},
		SecretToken: cfg.WebhookSecret,
	}

	settings := tele.Settings{
		Token:   cfg.Token,
		Poller:  webhook,
		OnError: makeErrorHandler(cfg.Logger),
	}

	teleBot, err := tele.NewBot(settings)
	if err != nil {
		return nil, err
	}

	b := &Bot{
		bot:     teleBot,
		storage: cfg.Storage,
		logger:  cfg.Logger,
	}

	// регистрация хендлеров
	b.registerHandlers()

	return b, nil
}

// Start запускает бота (блокирующий вызов).
func (b *Bot) Start() {
	b.logger.Info("запуск бота")
	b.bot.Start()
}

// Stop останавливает бота.
func (b *Bot) Stop() {
	b.logger.Info("остановка бота")
	b.bot.Stop()
}

// TeleBot возвращает внутренний экземпляр telebot (для тестов и shutdown).
func (b *Bot) TeleBot() *tele.Bot {
	return b.bot
}

// registerHandlers регистрирует все обработчики сообщений.
func (b *Bot) registerHandlers() {
	// группа для приватных команд (настройка)
	privateGroup := b.bot.Group()
	privateGroup.Use(PrivateOnly())

	privateGroup.Handle("/start", b.handleStart)
	privateGroup.Handle("/help", b.handleHelp)

	// команды с проверкой прав администратора
	adminGroup := b.bot.Group()
	adminGroup.Use(PrivateOnly())
	adminGroup.Use(AdminOnly(b.logger))

	// /setbot и /setsticker требуют парсинга chat_id из аргументов,
	// поэтому AdminOnly middleware будет применяться после парсинга
	privateGroup.Handle("/setbot", b.handleSetBot)
	privateGroup.Handle("/setsticker", b.handleSetSticker)
	privateGroup.Handle("/setstickerpack", b.handleSetStickerPack)
	privateGroup.Handle("/setlimits", b.handleSetLimits)
	privateGroup.Handle("/stats", b.handleStats)

	// все типы сообщений — единый роутер: приватные стикеры -> /setsticker flow, групповые -> спам-детекция
	for _, event := range []string{tele.OnText, tele.OnPhoto, tele.OnDocument, tele.OnSticker, tele.OnVideo, tele.OnAnimation} {
		b.bot.Handle(event, b.handleMessage)
	}
}

// makeErrorHandler создаёт обработчик ошибок telebot.
func makeErrorHandler(logger *slog.Logger) func(error, tele.Context) {
	return func(err error, c tele.Context) {
		var chatID int64
		var userID int64
		if c.Chat() != nil {
			chatID = c.Chat().ID
		}
		if c.Sender() != nil {
			userID = c.Sender().ID
		}
		logger.Error("telebot error", "error", err, "chat_id", chatID, "user_id", userID)
	}
}

// StickerAwait хранит состояние ожидания стикера для команды /setsticker.
type StickerAwait struct {
	UserID   int64
	ChatID   int64
	Expires  time.Time
	ChatName string
}
