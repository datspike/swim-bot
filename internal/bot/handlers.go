package bot

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/datspike/swim-bot/internal/storage"

	tele "gopkg.in/telebot.v3"
)

const spamReplyAutoDeleteTTL = time.Minute

// handleStart обрабатывает команду /start.
func (b *Bot) handleStart(c tele.Context) error {
	msg := `Привет! Я анти-спам бот.

Для настройки добавь меня в чат как администратора, затем:
1. /setbot <chat_id> @username — указать спам-бота
2. /setcommunityban <chat_id> on|off — включить автобан цитатного спама
3. /setspamlog <chat_id> <target_chat_id> — указать чат для логов community-ban
4. /setbotdelete <chat_id> @bot_username <seconds> — автоудалять прямые сообщения от bot-аккаунта

/help — подробная справка`

	return c.Send(msg)
}

// handleHelp обрабатывает команду /help.
func (b *Bot) handleHelp(c tele.Context) error {
	msg := `Полная справка по командам swim-bot

Базовые:
/start — краткое приветствие и быстрый старт.
/help — эта подробная справка.

Настройка детекции:
/setbot <chat_id> @username
  Назначает inline-бота, через которого считаем сообщения спамом (via_bot).
  Пример: /setbot -100123456789 @mlversebot

/setlimits <chat_id> [daily=N] [rb_size=N] [rb_threshold=N] [rb_steal=on|off]
  Настраивает лимиты:
  - daily: суточный лимит срабатываний до restrict,
  - rb_size: размер скользящего окна сообщений,
  - rb_threshold: порог спам-событий в окне для reactive-контекста,
  - rb_steal=off: отключает reactive-штраф ("воровство попытки").
  Пример: /setlimits -100123456789 daily=6 rb_size=30 rb_threshold=3 rb_steal=on

/getlimits <chat_id>
  Показывает текущие лимиты и связанные настройки.
  Пример: /getlimits -100123456789

/setspamdelete <chat_id> <seconds>
  TTL автоудаления спам-сообщений (через tracked bot) в секундах.
  0 = выключено.
  Пример: /setspamdelete -100123456789 60

/setbotdelete <chat_id> <bot_username> <seconds>
  Автоудаляет прямые сообщения от указанного bot-аккаунта через TTL.
  Работает только для message.sender.is_bot, не для inline via_bot.
  0 = удалить правило.
  Пример: /setbotdelete -100123456789 clown_alert_bot 60

/delbotdelete <chat_id> <bot_username>
  Удаляет правило автоудаления прямых bot-сообщений.
  Пример: /delbotdelete -100123456789 clown_alert_bot

/listbotdelete <chat_id>
  Показывает правила автоудаления прямых bot-сообщений.
  Пример: /listbotdelete -100123456789

Режимы и обслуживание:
/testmode <chat_id> on|off
  Включает/выключает тестовый режим:
  - добавляет в ответы отладочный префикс [ТЕСТ M/N rb:X/Y],
  - защищает админов от тестового restrict.
  Пример: /testmode -100123456789 on

/resetcounters <chat_id> [force=on]
  Сбрасывает дневные spam-счётчики и снимает выданные ботом дневные ограничения.
  По умолчанию доступно только в test mode.
  force=on — разрешает сброс вне test mode и отправляет уведомление в чат.
  Пример: /resetcounters -100123456789 force=on

/stats <chat_id>
  Статистика срабатываний: количество, последний триггер, статус активации.
  Пример: /stats -100123456789

Community-ban (цитатный спам):
/setcommunityban <chat_id> on|off
  Включает/выключает community-ban для подозрительных цитат из каналов.
  Пример: /setcommunityban -100123456789 on

/setspamlog <chat_id> <target_chat_id>
  Назначает чат для логов и копий community-ban кейсов.
  Пример: /setspamlog -100123456789 -100987654321

/communitybanstatus <chat_id>
  Показывает статус community-ban и текущий log-chat.
  Пример: /communitybanstatus -100123456789

Как получить chat_id:
1) Добавь @raw_data_bot в нужный чат.
2) Отправь любое сообщение.
3) Возьми числовой chat_id из ответа.

Важно:
- Все команды настройки выполняются в ЛС с этим ботом.
- Для указанного <chat_id> ты должен быть администратором/владельцем.`

	return c.Send(msg)
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
	if isMessageOlderThan(msg.Time(), b.maxMessageAge, b.activeScheduler().Now()) {
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

	if b.handleConfiguredBotMessageDelete(chatID, msg) {
		return nil
	}

	// детекция спама: via_bot
	isSpam := isTrackedBotSpam(msg, cfg.TrackedBot)

	// обновляем ring buffer (все сообщения)
	if msg.Sender != nil {
		rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
		rb.Push(isSpam, msg.Sender.ID)
	}

	if msg.Sender == nil {
		return nil
	}

	if isMTSQuestionnaireSpamCandidate(msg) {
		adminMember, adminErr := c.Bot().ChatMemberOf(c.Chat(), msg.Sender)
		if adminErr != nil {
			b.logger.Warn("mts questionnaire spam admin check failed", "chat_id", chatID, "user_id", msg.Sender.ID, "error", adminErr)
			return nil
		}
		if !isChatAdmin(adminMember.Role) {
			b.logger.Info("spam detected",
				"chat_id", chatID, "user_id", msg.Sender.ID,
				"trigger", "mts_questionnaire", "message_id", msg.ID,
				"test_mode", cfg.TestMode)
			return b.handleMTSQuestionnaireSpam(c, msg, cfg)
		}
	}

	if cfg.CommunityBanEnabled && isCommunityBanCandidate(msg) {
		adminMember, adminErr := c.Bot().ChatMemberOf(c.Chat(), msg.Sender)
		if adminErr != nil {
			b.logger.Warn("community ban admin check failed", "chat_id", chatID, "user_id", msg.Sender.ID, "error", adminErr)
			return nil
		}
		if shouldCommunityBan(msg, cfg, adminMember.Role) {
			b.logger.Info("spam detected",
				"chat_id", chatID, "user_id", msg.Sender.ID,
				"trigger", "community_ban", "message_id", msg.ID,
				"test_mode", cfg.TestMode)
			return b.handleCommunityBanDetection(c, msg, cfg)
		}
	}

	if !isSpam {
		return nil
	}

	// админы освобождены от спам-обработки
	adminMember, err := c.Bot().ChatMemberOf(c.Chat(), msg.Sender)
	if err != nil {
		b.logger.Warn("admin check failed in spam detection, skipping", "chat_id", chatID, "user_id", msg.Sender.ID, "error", err)
		return nil
	}
	if isChatAdmin(adminMember.Role) {
		b.handleAdminSpam(chatID, msg, adminMember.Role, cfg.SpamDeleteTTLSec)
		return nil
	}

	b.logger.Info("spam detected",
		"chat_id", chatID, "user_id", msg.Sender.ID,
		"trigger", "via_bot", "message_id", msg.ID,
		"test_mode", cfg.TestMode)

	return b.handleSpam(c, msg, cfg)
}

func (b *Bot) handleConfiguredBotMessageDelete(chatID int64, msg *tele.Message) bool {
	botUsername := botSenderUsername(msg)
	if botUsername == "" {
		return false
	}

	rule, err := b.storage.GetBotDeleteRule(chatID, botUsername)
	if err != nil {
		b.logger.Error("get bot delete rule failed", "chat_id", chatID, "bot_username", botUsername, "error", err)
		return true
	}
	if !shouldDeleteBotMessage(msg, rule) {
		return false
	}

	ttl := time.Duration(rule.TTLSec) * time.Second
	b.scheduleMessageDelete(chatID, msg.ID, ttl, "bot message")
	b.logger.Info("bot message delete scheduled",
		"chat_id", chatID, "bot_username", botUsername,
		"message_id", msg.ID, "ttl_sec", rule.TTLSec)
	return true
}

func (b *Bot) handleAdminSpam(chatID int64, msg *tele.Message, role tele.MemberStatus, ttlSec int) {
	if !shouldDeleteAdminSpam(role, ttlSec) {
		return
	}
	ttl := time.Duration(ttlSec) * time.Second
	b.scheduleSpamMessageDelete(chatID, msg.ID, ttl)
	b.logger.Info("admin spam delete scheduled",
		"chat_id", chatID, "user_id", msg.Sender.ID,
		"message_id", msg.ID, "ttl_sec", ttlSec)
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

	if result.AlreadyRestricted {
		ttl := time.Duration(cfg.SpamDeleteTTLSec) * time.Second
		if ttl > 0 {
			b.scheduleSpamMessageDelete(chatID, msg.ID, ttl)
		}
		return nil
	}

	var replyMsgID sql.NullInt64

	switch result.Action {
	case storage.ActionRestrict:
		b.restrictUser(c, msg, cfg)
		if err := b.storage.MarkKicked(chatID, msg.Sender.ID); err != nil {
			b.logger.Error("mark kicked failed", "chat_id", chatID, "user_id", msg.Sender.ID, "error", err)
		}

		// отправляем текст предупреждения если есть
		if result.Message != "" {
			if replyMsg := b.sendReply(c, msg, result.Message); replyMsg != nil {
				replyMsgID = sql.NullInt64{Int64: int64(replyMsg.ID), Valid: true}
			}
		}

	case storage.ActionWarning:
		if replyMsg := b.sendReply(c, msg, result.Message); replyMsg != nil {
			replyMsgID = sql.NullInt64{Int64: int64(replyMsg.ID), Valid: true}
			if shouldAutoDeleteSpamReply(result.Message) {
				b.scheduleSpamReplyDelete(chatID, replyMsg.ID)
			}
		}
	}

	ttl := time.Duration(cfg.SpamDeleteTTLSec) * time.Second
	if ttl > 0 {
		b.scheduleSpamMessageDelete(chatID, msg.ID, ttl)
	}

	logErr := b.storage.InsertActionLog(chatID, msg.Sender.ID, int64(msg.ID), replyMsgID, result.Context, result.Action)
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

func shouldAutoDeleteSpamReply(text string) bool {
	return strings.Contains(text, "Вы можете поплавать ещё")
}

// isMessageOlderThan проверяет возраст сообщения относительно текущего времени.
func isMessageOlderThan(messageTime time.Time, maxAge time.Duration, now time.Time) bool {
	return now.Sub(messageTime) > maxAge
}

// isTrackedBotSpam проверяет спам через отслеживаемого inline-бота.
func isTrackedBotSpam(msg *tele.Message, trackedBot string) bool {
	return msg != nil && msg.Sender != nil && msg.Via != nil && strings.EqualFold(msg.Via.Username, trackedBot)
}

// isChatAdmin проверяет роль администратора или владельца чата.
func isChatAdmin(role tele.MemberStatus) bool {
	return role == tele.Administrator || role == tele.Creator
}

// shouldDeleteAdminSpam проверяет необходимость удаления спама от админа.
func shouldDeleteAdminSpam(role tele.MemberStatus, ttlSec int) bool {
	return isChatAdmin(role) && ttlSec > 0
}

// botSenderUsername возвращает username прямого отправителя-бота.
func botSenderUsername(msg *tele.Message) string {
	if msg == nil || msg.Sender == nil || !msg.Sender.IsBot {
		return ""
	}
	username, err := storage.NormalizeBotUsername(msg.Sender.Username)
	if err != nil {
		return ""
	}
	return username
}

// shouldDeleteBotMessage проверяет применимость правила автоудаления к bot-сообщению.
func shouldDeleteBotMessage(msg *tele.Message, rule *storage.BotDeleteRule) bool {
	if rule == nil || rule.TTLSec <= 0 {
		return false
	}
	botUsername := botSenderUsername(msg)
	return botUsername != "" && botUsername == rule.BotUsername
}

func (b *Bot) scheduleSpamReplyDelete(chatID int64, messageID int) {
	b.activeScheduler().AfterFunc(spamReplyAutoDeleteTTL, func() {
		replyMsg := &tele.Message{
			ID:   messageID,
			Chat: &tele.Chat{ID: chatID},
		}
		if err := withRetry(func() error {
			return b.bot.Delete(replyMsg)
		}, b.logger); err != nil {
			b.logger.Warn("spam warning reply delete failed", "chat_id", chatID, "message_id", messageID, "error", err)
		}
	})
}

func (b *Bot) scheduleSpamMessageDelete(chatID int64, messageID int, ttl time.Duration) {
	b.scheduleMessageDelete(chatID, messageID, ttl, "spam message")
}

func (b *Bot) scheduleMessageDelete(chatID int64, messageID int, ttl time.Duration, reason string) {
	b.activeScheduler().AfterFunc(ttl, func() {
		message := &tele.Message{
			ID:   messageID,
			Chat: &tele.Chat{ID: chatID},
		}
		if err := withRetry(func() error {
			return b.bot.Delete(message)
		}, b.logger); err != nil {
			b.logger.Warn("message delete failed", "reason", reason, "chat_id", chatID, "message_id", messageID, "error", err)
		}
	})
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
		RestrictedUntil: endOfDayUTCAt(b.activeScheduler().Now()),
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

func (b *Bot) activeScheduler() botScheduler {
	if b.scheduler != nil {
		return b.scheduler
	}
	return realBotScheduler{}
}

// endOfDayUTCAt возвращает Unix timestamp полуночи следующего дня UTC.
// Если до конца суток < 30 секунд — берёт конец следующих суток,
// т.к. Telegram считает restrict < 30с бессрочным.
func endOfDayUTCAt(now time.Time) int64 {
	now = now.UTC()
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
