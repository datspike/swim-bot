package bot

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/datspike/swim-bot/internal/storage"

	tele "gopkg.in/telebot.v3"
)

// handleStart обрабатывает команду /start.
func (b *Bot) handleStart(c tele.Context) error {
	msg := `Привет! Я анти-спам бот.

Для настройки добавь меня в чат как администратора, затем:
1. /setbot <chat_id> @username — указать спам-бота

/help — подробная справка`

	return c.Send(msg)
}

// handleHelp обрабатывает команду /help.
func (b *Bot) handleHelp(c tele.Context) error {
	msg := `Команды:

/setbot <chat_id> @username
Указать, какого inline-бота отслеживать в чате.
Пример: /setbot -100123456789 @mlversebot

/setlimits <chat_id> [daily=N] [rb_size=N] [rb_threshold=N]
Настроить лимиты.
Пример: /setlimits -100123456789 daily=6 rb_size=30 rb_threshold=3

/getlimits <chat_id>
Показать текущие настройки лимитов.
Пример: /getlimits -100123456789

/testmode <chat_id> on|off
Включить/выключить тестовый режим (отладочный вывод [ТЕСТ M/N rb:X/Y], админы защищены от restrict).
Пример: /testmode -100123456789 on

/resetcounters <chat_id>
Сбросить все спам-счётчики за сегодня (только в тестовом режиме).
Пример: /resetcounters -100123456789

/stats <chat_id>
Показать статистику срабатываний.
Пример: /stats -100123456789

Как узнать chat_id:
1. Добавь @raw_data_bot в чат
2. Отправь любое сообщение
3. Бот покажет chat_id

Как это работает:
1. Добавь меня в чат как администратора
2. Настрой /setbot
3. Пользователь спамит -> предупреждения -> restrict до конца дня`

	return c.Send(msg)
}

// handleSetBot обрабатывает команду /setbot <chat_id> @username.
func (b *Bot) handleSetBot(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 {
		return c.Send("Формат: /setbot <chat_id> @username")
	}

	// парсинг chat_id
	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id. Используй числовой ID чата.")
	}

	username := args[1]

	// проверка прав администратора
	member, err := c.Bot().ChatMemberOf(&tele.Chat{ID: chatID}, c.Sender())
	if err != nil {
		b.logger.Warn("setbot admin check failed", "chat_id", chatID, "user_id", c.Sender().ID, "error", err)
		return c.Send(fmt.Sprintf("Не удалось проверить права. Возможно, я не добавлен в чат %d.", chatID))
	}

	if member.Role != tele.Administrator && member.Role != tele.Creator {
		return c.Send(fmt.Sprintf("Ты не администратор чата %d.", chatID))
	}

	// получаем название чата
	chat, err := c.Bot().ChatByID(chatID)
	chatTitle := fmt.Sprintf("%d", chatID)
	if err == nil && chat.Title != "" {
		chatTitle = chat.Title
	}

	// сохраняем в БД
	_, err = b.storage.UpsertTrackedBot(chatID, username)
	if err != nil {
		b.logger.Error("upsert tracked_bot failed", "chat_id", chatID, "error", err)
		return c.Send("Не удалось сохранить настройки. Попробуй позже.")
	}

	// нормализуем username для вывода
	username = strings.TrimPrefix(strings.ToLower(username), "@")

	b.logger.Info("tracked_bot updated", "chat_id", chatID, "tracked_bot", username, "by_user", c.Sender().ID)

	return c.Send(fmt.Sprintf("Готово! Отслеживаю @%s в чате %s.\nБот активен и готов к работе.", username, chatTitle))
}

// handleStats обрабатывает команду /stats <chat_id>.
func (b *Bot) handleStats(c tele.Context) error {
	args := c.Args()
	if len(args) < 1 {
		return c.Send("Формат: /stats <chat_id>")
	}

	// парсинг chat_id
	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id. Используй числовой ID чата.")
	}

	// проверка прав администратора
	member, err := c.Bot().ChatMemberOf(&tele.Chat{ID: chatID}, c.Sender())
	if err != nil {
		b.logger.Warn("stats admin check failed", "chat_id", chatID, "user_id", c.Sender().ID, "error", err)
		return c.Send(fmt.Sprintf("Не удалось проверить права. Возможно, я не добавлен в чат %d.", chatID))
	}

	if member.Role != tele.Administrator && member.Role != tele.Creator {
		return c.Send(fmt.Sprintf("Ты не администратор чата %d.", chatID))
	}

	// получаем название чата
	chat, err := c.Bot().ChatByID(chatID)
	chatTitle := fmt.Sprintf("%d", chatID)
	if err == nil && chat.Title != "" {
		chatTitle = chat.Title
	}

	// получаем статистику
	stats, err := b.storage.GetStats(chatID)
	if err != nil {
		b.logger.Error("get stats failed", "chat_id", chatID, "error", err)
		return c.Send("Не удалось получить статистику. Попробуй позже.")
	}

	// формируем ответ
	var status string
	if stats.IsActive {
		status = "активен"
	} else {
		status = "неактивен (настрой /setbot)"
	}

	var trackedBot string
	if stats.TrackedBot != "" {
		trackedBot = "@" + stats.TrackedBot
	} else {
		trackedBot = "не задан"
	}

	var lastTrigger string
	if stats.LastTrigger.Valid {
		lastTrigger = stats.LastTrigger.Time.Format("2006-01-02 15:04")
	} else {
		lastTrigger = "—"
	}

	msg := fmt.Sprintf(`Статистика чата %s:
- Всего срабатываний: %d
- Последнее: %s
- Отслеживаю: %s
- Статус: %s`, chatTitle, stats.TotalCount, lastTrigger, trackedBot, status)

	return c.Send(msg)
}

// handleSetLimits обрабатывает команду /setlimits <chat_id> [daily=N] [rb_size=N] [rb_threshold=N].
// Настраивает параметры rate limiting для чата.
func (b *Bot) handleSetLimits(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 {
		return c.Send("Формат: /setlimits <chat_id> [daily=N] [rb_size=N] [rb_threshold=N]")
	}

	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id. Используй числовой ID чата.")
	}

	// проверка прав администратора
	member, err := c.Bot().ChatMemberOf(&tele.Chat{ID: chatID}, c.Sender())
	if err != nil {
		b.logger.Warn("setlimits admin check failed", "chat_id", chatID, "user_id", c.Sender().ID, "error", err)
		return c.Send(fmt.Sprintf("Не удалось проверить права. Возможно, я не добавлен в чат %d.", chatID))
	}

	if member.Role != tele.Administrator && member.Role != tele.Creator {
		return c.Send(fmt.Sprintf("Ты не администратор чата %d.", chatID))
	}

	// получаем текущий конфиг для дефолтных значений
	cfg, err := b.storage.GetChatConfig(chatID)
	if err != nil || cfg == nil {
		return c.Send(fmt.Sprintf("Чат %d не настроен. Сначала выполни /setbot.", chatID))
	}

	// парсим key=value параметры
	daily := cfg.DailyLimit
	rbSize := cfg.RingBufferSize
	rbThreshold := cfg.RingBufferThreshold

	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			continue
		}
		val, parseErr := strconv.Atoi(parts[1])
		if parseErr != nil {
			return c.Send(fmt.Sprintf("Неверное значение: %s", arg))
		}

		switch parts[0] {
		case "daily":
			daily = val
		case "rb_size":
			rbSize = val
		case "rb_threshold":
			rbThreshold = val
		}
	}

	err = b.storage.UpdateRateLimitConfig(chatID, daily, rbSize, rbThreshold)
	if err != nil {
		b.logger.Error("update rate limit config failed", "chat_id", chatID, "error", err)
		return c.Send("Не удалось обновить настройки. Попробуй позже.")
	}

	b.logger.Info("rate limits updated", "chat_id", chatID, "daily", daily, "rb_size", rbSize, "rb_threshold", rbThreshold, "by_user", c.Sender().ID)

	return c.Send(fmt.Sprintf("Лимиты обновлены: daily=%d, rb_size=%d, rb_threshold=%d",
		daily, rbSize, rbThreshold))
}

// handleGetLimits обрабатывает команду /getlimits <chat_id>.
// Показывает текущие настройки лимитов для чата.
func (b *Bot) handleGetLimits(c tele.Context) error {
	args := c.Args()
	if len(args) < 1 {
		return c.Send("Формат: /getlimits <chat_id>")
	}

	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id. Используй числовой ID чата.")
	}

	// проверка прав администратора
	member, err := c.Bot().ChatMemberOf(&tele.Chat{ID: chatID}, c.Sender())
	if err != nil {
		b.logger.Warn("getlimits admin check failed", "chat_id", chatID, "user_id", c.Sender().ID, "error", err)
		return c.Send(fmt.Sprintf("Не удалось проверить права. Возможно, я не добавлен в чат %d.", chatID))
	}

	if member.Role != tele.Administrator && member.Role != tele.Creator {
		return c.Send(fmt.Sprintf("Ты не администратор чата %d.", chatID))
	}

	cfg, err := b.storage.GetChatConfig(chatID)
	if err != nil || cfg == nil {
		return c.Send(fmt.Sprintf("Чат %d не настроен. Сначала выполни /setbot.", chatID))
	}

	return c.Send(fmt.Sprintf(`Лимиты для чата %d:
- daily_limit: %d
- ring_buffer_size: %d
- ring_buffer_threshold: %d`, chatID, cfg.DailyLimit, cfg.RingBufferSize, cfg.RingBufferThreshold))
}

// handleTestMode обрабатывает команду /testmode <chat_id> on|off.
func (b *Bot) handleTestMode(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 {
		return c.Send("Использование: /testmode <chat_id> on|off")
	}

	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id. Используй числовой ID чата.")
	}

	mode := strings.ToLower(args[1])
	if mode != "on" && mode != "off" {
		return c.Send("Использование: /testmode <chat_id> on|off")
	}

	// проверка прав администратора
	member, err := c.Bot().ChatMemberOf(&tele.Chat{ID: chatID}, c.Sender())
	if err != nil {
		b.logger.Warn("testmode admin check failed", "chat_id", chatID, "user_id", c.Sender().ID, "error", err)
		return c.Send(fmt.Sprintf("Не удалось проверить права. Возможно, я не добавлен в чат %d.", chatID))
	}
	if member.Role != tele.Administrator && member.Role != tele.Creator {
		return c.Send(fmt.Sprintf("Ты не администратор чата %d.", chatID))
	}

	// проверяем что чат существует в конфиге
	cfg, err := b.storage.GetChatConfig(chatID)
	if err != nil || cfg == nil {
		return c.Send(fmt.Sprintf("Чат %d не найден в конфигурации.", chatID))
	}

	enabled := mode == "on"
	if err := b.storage.SetTestMode(chatID, enabled); err != nil {
		b.logger.Error("set test mode failed", "chat_id", chatID, "error", err)
		return c.Send("Не удалось обновить настройки. Попробуй позже.")
	}

	b.logger.Info("test mode updated", "chat_id", chatID, "enabled", enabled, "by_user", c.Sender().ID)

	if enabled {
		return c.Send(fmt.Sprintf("Тестовый режим включён для чата %d", chatID))
	}
	return c.Send(fmt.Sprintf("Тестовый режим выключен для чата %d", chatID))
}

// handleResetCounters обрабатывает команду /resetcounters <chat_id>.
// Сбрасывает все спам-счётчики за сегодня. Работает только в тестовом режиме.
func (b *Bot) handleResetCounters(c tele.Context) error {
	args := c.Args()
	if len(args) < 1 {
		return c.Send("Формат: /resetcounters <chat_id>")
	}

	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id.")
	}

	// проверка прав администратора
	member, err := c.Bot().ChatMemberOf(&tele.Chat{ID: chatID}, c.Sender())
	if err != nil {
		b.logger.Warn("resetcounters admin check failed", "chat_id", chatID, "user_id", c.Sender().ID, "error", err)
		return c.Send(fmt.Sprintf("Не удалось проверить права. Возможно, я не добавлен в чат %d.", chatID))
	}
	if member.Role != tele.Administrator && member.Role != tele.Creator {
		return c.Send(fmt.Sprintf("Ты не администратор чата %d.", chatID))
	}

	// проверяем что чат в тестовом режиме
	cfg, err := b.storage.GetChatConfig(chatID)
	if err != nil {
		return c.Send("Чат не настроен.")
	}
	if !cfg.TestMode {
		return c.Send("Сброс счётчиков доступен только в тестовом режиме.")
	}

	affected, err := b.storage.ResetSpamCounters(chatID)
	if err != nil {
		b.logger.Error("reset counters failed", "chat_id", chatID, "error", err)
		return c.Send("Не удалось сбросить счётчики.")
	}

	b.logger.Info("counters reset", "chat_id", chatID, "affected", affected, "by_user", c.Sender().ID)
	return c.Send(fmt.Sprintf("Сброшено %d счётчиков для чата %d", affected, chatID))
}

// handleMessage — единый роутер для всех типов сообщений.
// Групповые сообщения -> спам-детекция.
func (b *Bot) handleMessage(c tele.Context) error {
	return b.handleSpamDetection(c)
}

// handleSpamDetection обрабатывает сообщения в групповых чатах для обнаружения спама.
// Пропускает старые сообщения (FR-011).
func (b *Bot) handleSpamDetection(c tele.Context) error {
	msg := c.Message()
	if msg == nil {
		return nil
	}

	// пропускаем старые сообщения — защита от webhook backlog (FR-011)
	if time.Since(msg.Time()) > b.maxMessageAge {
		return nil
	}

	chatID := c.Chat().ID

	// получаем конфиг чата
	cfg, err := b.storage.GetChatConfig(chatID)
	if err != nil {
		b.logger.Error("get chat config failed", "chat_id", chatID, "error", err)
		return nil
	}

	if cfg == nil || !cfg.IsActive {
		return nil
	}

	// детекция спама: via_bot
	isSpam := false
	if msg.Via != nil && strings.ToLower(msg.Via.Username) == cfg.TrackedBot {
		isSpam = true
	}

	// обновляем ring buffer (все сообщения)
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rb.Push(isSpam, msg.Sender.ID)

	if !isSpam {
		return nil
	}

	// админы освобождены от спам-обработки
	adminMember, err := c.Bot().ChatMemberOf(c.Chat(), msg.Sender)
	if err != nil {
		b.logger.Warn("admin check failed in spam detection, skipping", "chat_id", chatID, "user_id", msg.Sender.ID, "error", err)
		return nil
	}
	if adminMember.Role == tele.Administrator || adminMember.Role == tele.Creator {
		return nil
	}

	b.logger.Info("spam detected",
		"chat_id", chatID, "user_id", msg.Sender.ID,
		"trigger", "via_bot", "message_id", msg.ID,
		"test_mode", cfg.TestMode)

	return b.handleSpam(c, msg, cfg)
}

// handleSpam обрабатывает спам (ring buffer + restrict).
func (b *Bot) handleSpam(c tele.Context, msg *tele.Message, cfg *storage.ChatConfig) error {
	chatID := c.Chat().ID

	result, err := b.processSpam(chatID, msg.Sender.ID, cfg)
	if err != nil {
		b.logger.Error("process spam failed", "chat_id", chatID, "error", err)
		return nil
	}

	// в тестовом режиме дописываем счётчик M/N и rb M/N к каждому сообщению
	if cfg.TestMode && result.Message != "" {
		result.Message = fmt.Sprintf("[ТЕСТ %d/%d rb:%d/%d] %s",
			result.Count, result.Limit, result.RBSpamCount, result.RBThreshold, result.Message)
	}

	var replyMsgID sql.NullInt64

	switch result.Action {
	case storage.ActionRestrict:
		b.restrictUser(c, msg, cfg)
		_ = b.storage.MarkKicked(chatID, msg.Sender.ID)

		// отправляем текст предупреждения если есть
		if result.Message != "" {
			if replyMsg := b.sendReply(c, msg, result.Message); replyMsg != nil {
				replyMsgID = sql.NullInt64{Int64: int64(replyMsg.ID), Valid: true}
			}
		}

	case storage.ActionWarning:
		if replyMsg := b.sendReply(c, msg, result.Message); replyMsg != nil {
			replyMsgID = sql.NullInt64{Int64: int64(replyMsg.ID), Valid: true}
		}
	}

	ctx := b.detectContext(chatID, msg.Sender.ID, cfg)

	logErr := b.storage.InsertActionLog(chatID, msg.Sender.ID, int64(msg.ID), replyMsgID, ctx, result.Action)
	if logErr != nil {
		b.logger.Error("insert action log failed", "chat_id", chatID, "error", logErr)
	}

	return nil
}

// sendReply отправляет текстовый ответ с retry при FloodError.
func (b *Bot) sendReply(c tele.Context, msg *tele.Message, text string) *tele.Message {
	var replyMsg *tele.Message
	err := withRetry(func() error {
		var e error
		replyMsg, e = c.Bot().Reply(msg, text)
		return e
	}, b.logger)
	if err != nil {
		b.logger.Error("send reply failed", "chat_id", c.Chat().ID, "error", err)
		return nil
	}
	return replyMsg
}

// restrictUser ограничивает пользователя (can_send_other_messages: false) до конца суток UTC.
// В тестовом режиме: админы получают текстовое сообщение, не-админы — реальный restrict.
func (b *Bot) restrictUser(c tele.Context, msg *tele.Message, cfg *storage.ChatConfig) {
	chatID := c.Chat().ID

	if cfg.TestMode {
		// проверяем, является ли пользователь админом
		member, err := c.Bot().ChatMemberOf(c.Chat(), msg.Sender)
		if err == nil && (member.Role == tele.Administrator || member.Role == tele.Creator) {
			// админ в тестовом режиме — только сообщение, не рестриктим
			testMsg := fmt.Sprintf("[ТЕСТ] Был бы restrict: %s", displayName(msg.Sender))
			b.sendReply(c, msg, testMsg)
			b.logger.Info("test restrict (admin)", "chat_id", chatID, "user_id", msg.Sender.ID, "display_name", displayName(msg.Sender))
			return
		}
		// не-админ — рестриктим реально (даже в тестовом режиме)
	}

	// реальный restrict
	rights := tele.NoRestrictions()
	rights.CanSendOther = false // блокируем инлайн-ботов, стикеры, GIF
	rights.Independent = true   // не затрагиваем остальные права

	chatMember := &tele.ChatMember{
		User:            msg.Sender,
		Rights:          rights,
		RestrictedUntil: endOfDayUTC(),
	}

	err := withRetry(func() error {
		return c.Bot().Restrict(c.Chat(), chatMember)
	}, b.logger)
	if err != nil {
		b.logger.Error("restrict failed", "chat_id", chatID, "user_id", msg.Sender.ID, "error", err)
		return
	}

	b.logger.Info("user restricted", "chat_id", chatID, "user_id", msg.Sender.ID, "until", chatMember.RestrictedUntil, "test_mode", cfg.TestMode)
}

// endOfDayUTC возвращает Unix timestamp полуночи следующего дня UTC.
// Если до конца суток < 30 секунд — берёт конец следующих суток,
// т.к. Telegram считает restrict < 30с бессрочным.
func endOfDayUTC() int64 {
	now := time.Now().UTC()
	endOfDay := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	if endOfDay.Sub(now) < 30*time.Second {
		endOfDay = endOfDay.Add(24 * time.Hour)
	}
	return endOfDay.Unix()
}

// displayName возвращает отображаемое имя пользователя.
// Формат: "FirstName LastName" или "@Username" или "Unknown".
func displayName(user *tele.User) string {
	name := user.FirstName
	if user.LastName != "" {
		name += " " + user.LastName
	}
	if name == "" && user.Username != "" {
		name = "@" + user.Username
	}
	if name == "" {
		name = "Unknown"
	}
	return name
}
