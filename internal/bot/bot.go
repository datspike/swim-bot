package bot

import (
	"log/slog"
	"time"

	"github.com/datspike/swim-bot/internal/storage"

	tele "gopkg.in/telebot.v3"
)

// defaultMaxMessageAge — максимальный возраст сообщения для обработки (FR-011).
const defaultMaxMessageAge = 30 * time.Second

// Bot содержит инициализированного Telegram бота.
type Bot struct {
	bot           *tele.Bot
	storage       *storage.Storage
	logger        *slog.Logger
	ringBuffers   ChatRingBuffers // скользящие окна сообщений per-chat
	maxMessageAge time.Duration   // порог пропуска старых сообщений (webhook backlog)
}

// Config содержит параметры для создания бота.
type Config struct {
	Token         string
	WebhookURL    string
	WebhookSecret string
	Port          int
	MaxMessageAge time.Duration // 0 = defaultMaxMessageAge (30s)
	Storage       *storage.Storage
	Logger        *slog.Logger
}

// New создаёт и настраивает Telegram бота с webhook poller.
func New(cfg Config) (*Bot, error) {
	settings := tele.Settings{
		Token:   cfg.Token,
		Poller:  newSafeWebhook(cfg),
		OnError: makeErrorHandler(cfg.Logger),
	}

	teleBot, err := tele.NewBot(settings)
	if err != nil {
		return nil, err
	}

	maxAge := cfg.MaxMessageAge
	if maxAge == 0 {
		maxAge = defaultMaxMessageAge
	}

	b := &Bot{
		bot:           teleBot,
		storage:       cfg.Storage,
		logger:        cfg.Logger,
		maxMessageAge: maxAge,
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

	// команды с проверкой прав администратора (парсинг chat_id внутри хендлера)
	privateGroup.Handle("/setbot", b.handleSetBot)
	privateGroup.Handle("/setlimits", b.handleSetLimits)
	privateGroup.Handle("/getlimits", b.handleGetLimits)
	privateGroup.Handle("/stats", b.handleStats)
	privateGroup.Handle("/testmode", b.handleTestMode)
	privateGroup.Handle("/resetcounters", b.handleResetCounters)
	privateGroup.Handle("/setcommunityban", b.handleSetCommunityBan)
	privateGroup.Handle("/setspamlog", b.handleSetSpamLog)
	privateGroup.Handle("/communitybanstatus", b.handleCommunityBanStatus)
	privateGroup.Handle("/setspamdelete", b.handleSetSpamDelete)

	// все типы сообщений — единый роутер: спам-детекция
	for _, event := range []string{tele.OnText, tele.OnPhoto, tele.OnDocument, tele.OnSticker, tele.OnVideo, tele.OnAnimation} {
		b.bot.Handle(event, b.handleMessage)
	}
}

// makeErrorHandler создаёт обработчик ошибок telebot.
func makeErrorHandler(logger *slog.Logger) func(error, tele.Context) {
	return func(err error, c tele.Context) {
		var chatID int64
		var userID int64
		if c != nil {
			if c.Chat() != nil {
				chatID = c.Chat().ID
			}
			if c.Sender() != nil {
				userID = c.Sender().ID
			}
		}
		logger.Error("telebot error", "error", err, "chat_id", chatID, "user_id", userID)
	}
}
