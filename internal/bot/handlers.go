package bot

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

// stickerAwaits хранит состояния ожидания стикера (ключ: UserID).
var stickerAwaits = sync.Map{}

// handleStart обрабатывает команду /start.
func (b *Bot) handleStart(c tele.Context) error {
	msg := `Привет! Я анти-спам бот.

Для настройки добавь меня в чат как администратора, затем:
1. /setbot <chat_id> @username — указать спам-бота
2. /setsticker <chat_id> — указать стикер (отправь стикер следующим сообщением)

/help — подробная справка`

	return c.Send(msg)
}

// handleHelp обрабатывает команду /help.
func (b *Bot) handleHelp(c tele.Context) error {
	msg := `Команды:

/setbot <chat_id> @username
Указать, какого inline-бота отслеживать в чате.
Пример: /setbot -100123456789 @mlversebot

/setsticker <chat_id>
Указать стикер для ответа. После команды отправь стикер.
Пример: /setsticker -100123456789

/stats <chat_id>
Показать статистику срабатываний.
Пример: /stats -100123456789

Как узнать chat_id:
1. Добавь @raw_data_bot в чат
2. Отправь любое сообщение
3. Бот покажет chat_id

Как это работает:
1. Добавь меня в чат как администратора
2. Настрой /setbot и /setsticker
3. Когда кто-то использует спам-бота, я отвечу стикером`

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

	return c.Send(fmt.Sprintf("Готово! Отслеживаю @%s в чате %s.\nТеперь отправь /setsticker %d и стикер.", username, chatTitle, chatID))
}

// handleSetSticker обрабатывает команду /setsticker <chat_id>.
func (b *Bot) handleSetSticker(c tele.Context) error {
	args := c.Args()
	if len(args) < 1 {
		return c.Send("Формат: /setsticker <chat_id>")
	}

	// парсинг chat_id
	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id. Используй числовой ID чата.")
	}

	// проверка прав администратора
	member, err := c.Bot().ChatMemberOf(&tele.Chat{ID: chatID}, c.Sender())
	if err != nil {
		b.logger.Warn("setsticker admin check failed", "chat_id", chatID, "user_id", c.Sender().ID, "error", err)
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

	// устанавливаем ожидание стикера
	await := StickerAwait{
		UserID:   c.Sender().ID,
		ChatID:   chatID,
		Expires:  time.Now().Add(60 * time.Second),
		ChatName: chatTitle,
	}
	stickerAwaits.Store(c.Sender().ID, await)

	b.logger.Info("sticker await set", "user_id", c.Sender().ID, "chat_id", chatID)

	return c.Send(fmt.Sprintf("Отправь стикер, который я буду использовать в чате %s.", chatTitle))
}

// handleStickerMessage обрабатывает входящие стикеры (для /setsticker flow).
func (b *Bot) handleStickerMessage(c tele.Context) error {
	// проверяем, ожидаем ли стикер от этого пользователя
	val, ok := stickerAwaits.Load(c.Sender().ID)
	if !ok {
		return nil // не ожидаем стикер от этого пользователя
	}

	await, ok := val.(StickerAwait)
	if !ok {
		return nil
	}

	// проверяем TTL
	if time.Now().After(await.Expires) {
		stickerAwaits.Delete(c.Sender().ID)
		return nil
	}

	// удаляем ожидание
	stickerAwaits.Delete(c.Sender().ID)

	// получаем FileID стикера
	sticker := c.Message().Sticker
	if sticker == nil {
		return c.Send(fmt.Sprintf("Ожидал стикер. Попробуй ещё раз: /setsticker %d", await.ChatID))
	}

	// сохраняем в БД
	err := b.storage.UpsertStickerFileID(await.ChatID, sticker.FileID)
	if err != nil {
		b.logger.Error("upsert sticker_file_id failed", "chat_id", await.ChatID, "error", err)
		return c.Send("Не удалось сохранить стикер. Попробуй позже.")
	}

	b.logger.Info("sticker saved", "chat_id", await.ChatID, "sticker_file_id", sticker.FileID, "by_user", c.Sender().ID)

	return c.Send(fmt.Sprintf("Стикер сохранён! Бот активен в чате %s.", await.ChatName))
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
		status = "неактивен (настрой /setbot и /setsticker)"
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

// handleMessage — единый роутер для всех типов сообщений.
// Приватные стикеры с ожиданием -> /setsticker flow, групповые -> спам-детекция.
func (b *Bot) handleMessage(c tele.Context) error {
	// приватный чат + стикер + ожидание -> /setsticker flow
	if c.Chat() != nil && c.Chat().Type == tele.ChatPrivate && c.Message() != nil && c.Message().Sticker != nil {
		if _, ok := stickerAwaits.Load(c.Sender().ID); ok {
			return b.handleStickerMessage(c)
		}
		return nil
	}

	return b.handleSpamDetection(c)
}

// handleSpamDetection обрабатывает сообщения в групповых чатах для обнаружения спама.
func (b *Bot) handleSpamDetection(c tele.Context) error {
	msg := c.Message()
	if msg == nil {
		return nil
	}

	// проверяем наличие via_bot
	if msg.Via == nil {
		return nil
	}

	chatID := c.Chat().ID

	// получаем конфиг чата
	cfg, err := b.storage.GetChatConfig(chatID)
	if err != nil {
		b.logger.Error("get chat config failed", "chat_id", chatID, "error", err)
		return nil
	}

	// проверяем, настроен ли бот для этого чата
	if cfg == nil || !cfg.IsActive {
		return nil
	}

	// сравниваем username via_bot с отслеживаемым
	viaUsername := strings.ToLower(msg.Via.Username)
	if viaUsername != cfg.TrackedBot {
		return nil
	}

	b.logger.Info("spam detected", "chat_id", chatID, "user_id", msg.Sender.ID, "via_bot", viaUsername, "message_id", msg.ID)

	// отправляем стикер в ответ
	sticker := &tele.Sticker{File: tele.File{FileID: cfg.StickerFileID}}
	replyMsg, err := c.Bot().Reply(msg, sticker)

	// логируем срабатывание
	var replyMsgID sql.NullInt64
	if err == nil && replyMsg != nil {
		replyMsgID = sql.NullInt64{Int64: int64(replyMsg.ID), Valid: true}
	} else {
		b.logger.Error("send sticker failed", "chat_id", chatID, "error", err)
	}

	logErr := b.storage.InsertActionLog(chatID, msg.Sender.ID, int64(msg.ID), replyMsgID)
	if logErr != nil {
		b.logger.Error("insert action log failed", "chat_id", chatID, "error", logErr)
	}

	return nil
}

// StartStickerAwaitCleanup запускает горутину очистки просроченных ожиданий стикеров.
func StartStickerAwaitCleanup() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now()
			stickerAwaits.Range(func(key, value any) bool {
				await, ok := value.(StickerAwait)
				if ok && now.After(await.Expires) {
					stickerAwaits.Delete(key)
				}
				return true
			})
		}
	}()
}
