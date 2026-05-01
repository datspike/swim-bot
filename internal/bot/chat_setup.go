package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/datspike/swim-bot/internal/chatconfig"
	"github.com/datspike/swim-bot/internal/storage"

	tele "gopkg.in/telebot.v3"
)

const unrestrictStatusClearDelay = 70 * time.Second

type resetRestrictionsResult struct {
	Candidates int
	Succeeded  int
	Failed     int
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

	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "setbot"); !ok {
		return c.Send(response)
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
	chatID, ok, response := parseChatIDArg(args, "Формат: /stats <chat_id>", "Неверный chat_id. Используй числовой ID чата.")
	if !ok {
		return c.Send(response)
	}
	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "stats"); !ok {
		return c.Send(response)
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

// handleSetLimits обрабатывает команду /setlimits <chat_id> [daily=N] [rb_size=N] [rb_threshold=N] [rb_steal=on|off].
// Настраивает параметры rate limiting для чата.
func (b *Bot) handleSetLimits(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 {
		return c.Send("Формат: /setlimits <chat_id> [daily=N] [rb_size=N] [rb_threshold=N] [rb_steal=on|off]")
	}

	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id. Используй числовой ID чата.")
	}

	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "setlimits"); !ok {
		return c.Send(response)
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
	rbStealExplicit := false

	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		case "daily":
			val, parseErr := strconv.Atoi(parts[1])
			if parseErr != nil {
				return c.Send(fmt.Sprintf("Неверное значение: %s", arg))
			}
			daily = val
		case "rb_size":
			val, parseErr := strconv.Atoi(parts[1])
			if parseErr != nil {
				return c.Send(fmt.Sprintf("Неверное значение: %s", arg))
			}
			rbSize = val
		case "rb_threshold":
			val, parseErr := strconv.Atoi(parts[1])
			if parseErr != nil {
				return c.Send(fmt.Sprintf("Неверное значение: %s", arg))
			}
			rbThreshold = val
		case "rb_steal":
			mode := strings.ToLower(parts[1])
			switch mode {
			case "off":
				rbThreshold = 0
				rbStealExplicit = true
			case "on":
				if rbThreshold <= 0 {
					rbThreshold = 2
				}
				rbStealExplicit = true
			default:
				return c.Send("Неверное значение rb_steal. Используй on или off.")
			}
		}
	}

	if validationErr := validateRateLimitConfig(daily, rbSize, rbThreshold); validationErr != nil {
		return c.Send(validationErr.Error())
	}

	err = b.storage.UpdateRateLimitConfig(chatID, daily, rbSize, rbThreshold)
	if err != nil {
		b.logger.Error("update rate limit config failed", "chat_id", chatID, "error", err)
		return c.Send("Не удалось обновить настройки. Попробуй позже.")
	}

	b.logger.Info("rate limits updated", "chat_id", chatID, "daily", daily, "rb_size", rbSize, "rb_threshold", rbThreshold, "by_user", c.Sender().ID)

	status := "on"
	if rbThreshold <= 0 {
		status = "off"
	}
	if rbStealExplicit {
		return c.Send(fmt.Sprintf("Лимиты обновлены: daily=%d, rb_size=%d, rb_threshold=%d, rb_steal=%s",
			daily, rbSize, rbThreshold, status))
	}
	return c.Send(fmt.Sprintf("Лимиты обновлены: daily=%d, rb_size=%d, rb_threshold=%d (rb_steal=%s)",
		daily, rbSize, rbThreshold, status))
}

// handleGetLimits обрабатывает команду /getlimits <chat_id>.
// Показывает текущие настройки лимитов для чата.
func (b *Bot) handleGetLimits(c tele.Context) error {
	args := c.Args()
	chatID, ok, response := parseChatIDArg(args, "Формат: /getlimits <chat_id>", "Неверный chat_id. Используй числовой ID чата.")
	if !ok {
		return c.Send(response)
	}
	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "getlimits"); !ok {
		return c.Send(response)
	}

	cfg, err := b.storage.GetChatConfig(chatID)
	if err != nil || cfg == nil {
		return c.Send(fmt.Sprintf("Чат %d не настроен. Сначала выполни /setbot.", chatID))
	}

	return c.Send(fmt.Sprintf(`Лимиты для чата %d:
- daily_limit: %d
- ring_buffer_size: %d
- ring_buffer_threshold: %d
- rb_steal: %s`, chatID, cfg.DailyLimit, cfg.RingBufferSize, cfg.RingBufferThreshold, rbStealStatus(cfg.RingBufferThreshold)))
}

// handleTestMode обрабатывает команду /testmode <chat_id> on|off.
func (b *Bot) handleTestMode(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 {
		return c.Send("Использование: /testmode <chat_id> on|off")
	}

	chatID, ok, response := parseChatIDArg(args, "Использование: /testmode <chat_id> on|off", "Неверный chat_id. Используй числовой ID чата.")
	if !ok {
		return c.Send(response)
	}

	mode := strings.ToLower(args[1])
	if mode != "on" && mode != "off" {
		return c.Send("Использование: /testmode <chat_id> on|off")
	}

	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "testmode"); !ok {
		return c.Send(response)
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

func parseChatIDArg(args []string, usageText, invalidText string) (chatID int64, ok bool, response string) {
	if len(args) < 1 {
		return 0, false, usageText
	}
	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return 0, false, invalidText
	}
	return chatID, true, ""
}

func rbStealStatus(rbThreshold int) string {
	return chatconfig.RBStealStatus(rbThreshold)
}

func validateRateLimitConfig(daily, rbSize, rbThreshold int) error {
	return chatconfig.ValidateRateLimitConfig(daily, rbSize, rbThreshold)
}

// handleResetCounters обрабатывает команду /resetcounters <chat_id> [force=on].
// Сбрасывает дневные спам-счётчики и снимает выданные ботом ограничения.
// При force=on можно сбрасывать всегда.
func (b *Bot) handleResetCounters(c tele.Context) error {
	args := c.Args()
	if len(args) < 1 {
		return c.Send("Формат: /resetcounters <chat_id> [force=on]")
	}

	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id.")
	}

	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "resetcounters"); !ok {
		return c.Send(response)
	}

	forceReset := false
	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[0] == "force" {
			forceReset = strings.EqualFold(parts[1], "on")
		}
	}

	// проверяем что чат в тестовом режиме
	cfg, err := b.storage.GetChatConfig(chatID)
	if err != nil {
		return c.Send("Чат не настроен.")
	}
	if cfg == nil {
		return c.Send("Чат не настроен.")
	}
	if !cfg.TestMode && !forceReset {
		return c.Send("Сброс счётчиков доступен только в тестовом режиме.")
	}

	resetDate := b.activeScheduler().Now().UTC().Format(time.DateOnly)
	kickedUsers, err := b.storage.ListKickedSpamCounterUsers(chatID, resetDate)
	if err != nil {
		b.logger.Error("list kicked spam counter users failed", "chat_id", chatID, "date", resetDate, "error", err)
		return c.Send("Не удалось получить список ограничений для сброса.")
	}

	restrictions := b.unrestrictUsers(chatID, kickedUsers)

	affected, err := b.storage.ResetSpamCountersForDate(chatID, resetDate)
	if err != nil {
		b.logger.Error("reset counters failed", "chat_id", chatID, "date", resetDate, "error", err)
		return c.Send("Не удалось сбросить счётчики.")
	}

	notifyText := "Ого, повезло, счетчики плавания на сегодня сброшены"
	if _, sendErr := c.Bot().Send(&tele.Chat{ID: chatID}, notifyText); sendErr != nil {
		b.logger.Warn("reset counters notify failed", "chat_id", chatID, "error", sendErr)
	}

	b.logger.Info(
		"counters reset",
		"chat_id", chatID,
		"date", resetDate,
		"affected", affected,
		"unrestrict_candidates", restrictions.Candidates,
		"unrestrict_succeeded", restrictions.Succeeded,
		"unrestrict_failed", restrictions.Failed,
		"by_user", c.Sender().ID,
		"force", forceReset,
	)
	return c.Send(formatResetCountersResponse(affected, chatID, restrictions))
}

// buildUnrestrictChatMember создаёт параметры снятия дневных ограничений пользователя.
func buildUnrestrictChatMember(userID int64) *tele.ChatMember {
	return buildUnrestrictChatMemberAt(userID, realBotScheduler{}.Now())
}

func buildUnrestrictUntilAt(now time.Time) int64 {
	return now.Add(unrestrictStatusClearDelay).Unix()
}

func buildUnrestrictChatMemberAt(userID int64, now time.Time) *tele.ChatMember {
	rights := tele.NoRestrictions()
	rights.Independent = true

	return &tele.ChatMember{
		User:   &tele.User{ID: userID},
		Rights: rights,
		// Telegram оставляет status=restricted при until_date=0, поэтому даём короткий срок.
		RestrictedUntil: buildUnrestrictUntilAt(now),
	}
}

// unrestrictUser снимает Telegram-ограничения пользователя с retry при FloodError.
func (b *Bot) unrestrictUser(chatID, userID int64) error {
	return withRetry(func() error {
		member := buildUnrestrictChatMemberAt(userID, b.activeScheduler().Now())
		return b.bot.Restrict(&tele.Chat{ID: chatID}, member)
	}, b.logger)
}

// unrestrictUsers снимает ограничения с кандидатов и продолжает при частичных ошибках.
func (b *Bot) unrestrictUsers(chatID int64, userIDs []int64) resetRestrictionsResult {
	uniqueUserIDs := uniqueInt64s(userIDs)
	result := resetRestrictionsResult{Candidates: len(uniqueUserIDs)}

	for _, userID := range uniqueUserIDs {
		if err := b.unrestrictUser(chatID, userID); err != nil {
			result.Failed++
			b.logger.Warn("unrestrict user failed", "chat_id", chatID, "user_id", userID, "error", err)
			continue
		}
		result.Succeeded++
	}

	return result
}

// uniqueInt64s возвращает список чисел без повторов с сохранением порядка.
func uniqueInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	uniqueValues := make([]int64, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		uniqueValues = append(uniqueValues, value)
	}
	return uniqueValues
}

// formatResetCountersResponse формирует сводку сброса для администратора.
func formatResetCountersResponse(affected, chatID int64, restrictions resetRestrictionsResult) string {
	return fmt.Sprintf(
		"Сброшено %d счётчиков для чата %d. Ограничения: найдено %d, снято %d, ошибок %d.",
		affected,
		chatID,
		restrictions.Candidates,
		restrictions.Succeeded,
		restrictions.Failed,
	)
}

// handleSetCommunityBan обрабатывает команду /setcommunityban <chat_id> on|off.
func (b *Bot) handleSetCommunityBan(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 {
		return c.Send("Использование: /setcommunityban <chat_id> on|off")
	}

	chatID, ok, response := parseChatIDArg(args, "Использование: /setcommunityban <chat_id> on|off", "Неверный chat_id. Используй числовой ID чата.")
	if !ok {
		return c.Send(response)
	}

	mode := strings.ToLower(args[1])
	if mode != "on" && mode != "off" {
		return c.Send("Использование: /setcommunityban <chat_id> on|off")
	}

	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "setcommunityban"); !ok {
		return c.Send(response)
	}

	enabled := mode == "on"
	if err := b.storage.SetCommunityBanEnabled(chatID, enabled); err != nil {
		b.logger.Error("set community ban failed", "chat_id", chatID, "enabled", enabled, "error", err)
		return c.Send("Не удалось обновить настройки community-ban.")
	}

	b.logger.Info("community ban updated", "chat_id", chatID, "enabled", enabled, "by_user", c.Sender().ID)
	if enabled {
		return c.Send(fmt.Sprintf("Community-ban включён для чата %d", chatID))
	}
	return c.Send(fmt.Sprintf("Community-ban выключен для чата %d", chatID))
}

// handleSetSpamLog обрабатывает команду /setspamlog <chat_id> <target_chat_id>.
func (b *Bot) handleSetSpamLog(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 {
		return c.Send("Использование: /setspamlog <chat_id> <target_chat_id>")
	}

	chatID, ok, response := parseChatIDArg(args, "Использование: /setspamlog <chat_id> <target_chat_id>", "Неверный chat_id. Используй числовой ID чата.")
	if !ok {
		return c.Send(response)
	}
	targetChatID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return c.Send("Неверный target_chat_id. Используй числовой ID чата.")
	}

	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "setspamlog"); !ok {
		return c.Send(response)
	}

	if err := b.storage.SetSpamLogChatID(chatID, targetChatID); err != nil {
		b.logger.Error("set spam log failed", "chat_id", chatID, "target_chat_id", targetChatID, "error", err)
		return c.Send("Не удалось сохранить чат для логов.")
	}

	b.logger.Info("spam log chat updated", "chat_id", chatID, "target_chat_id", targetChatID, "by_user", c.Sender().ID)
	return c.Send(fmt.Sprintf("Лог community-ban для чата %d -> %d", chatID, targetChatID))
}

// handleCommunityBanStatus обрабатывает команду /communitybanstatus <chat_id>.
func (b *Bot) handleCommunityBanStatus(c tele.Context) error {
	args := c.Args()
	chatID, ok, response := parseChatIDArg(args, "Использование: /communitybanstatus <chat_id>", "Неверный chat_id. Используй числовой ID чата.")
	if !ok {
		return c.Send(response)
	}
	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "communitybanstatus"); !ok {
		return c.Send(response)
	}

	cfg, err := b.storage.GetChatConfig(chatID)
	if err != nil {
		b.logger.Error("communitybanstatus get config failed", "chat_id", chatID, "error", err)
		return c.Send("Не удалось получить настройки.")
	}
	if cfg == nil {
		return c.Send(fmt.Sprintf("Чат %d не настроен.", chatID))
	}

	status := "off"
	if cfg.CommunityBanEnabled {
		status = "on"
	}
	logChat := "не задан"
	if cfg.SpamLogChatID != 0 {
		logChat = strconv.FormatInt(cfg.SpamLogChatID, 10)
	}

	return c.Send(fmt.Sprintf("Community-ban для чата %d:\n- статус: %s\n- log chat: %s", chatID, status, logChat))
}

// handleSetSpamDelete обрабатывает команду /setspamdelete <chat_id> <seconds>.
func (b *Bot) handleSetSpamDelete(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 {
		return c.Send("Использование: /setspamdelete <chat_id> <seconds>")
	}
	chatID, ok, response := parseChatIDArg(args, "Использование: /setspamdelete <chat_id> <seconds>", "Неверный chat_id. Используй числовой ID чата.")
	if !ok {
		return c.Send(response)
	}
	ttlSec, err := strconv.Atoi(args[1])
	if err != nil || ttlSec < 0 {
		return c.Send("Неверный seconds. Укажи целое число >= 0.")
	}

	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "setspamdelete"); !ok {
		return c.Send(response)
	}

	if err := b.storage.SetSpamDeleteTTL(chatID, ttlSec); err != nil {
		b.logger.Error("set spam delete ttl failed", "chat_id", chatID, "ttl_sec", ttlSec, "error", err)
		return c.Send("Не удалось обновить TTL автоудаления.")
	}

	b.logger.Info("spam delete ttl updated", "chat_id", chatID, "ttl_sec", ttlSec, "by_user", c.Sender().ID)
	if ttlSec == 0 {
		return c.Send(fmt.Sprintf("Автоудаление спам-сообщений для чата %d выключено.", chatID))
	}
	return c.Send(fmt.Sprintf("Автоудаление спам-сообщений для чата %d: %d сек.", chatID, ttlSec))
}

// handleSetBotDelete обрабатывает команду /setbotdelete <chat_id> <bot_username> <seconds>.
func (b *Bot) handleSetBotDelete(c tele.Context) error {
	args := c.Args()
	if len(args) < 3 {
		return c.Send("Использование: /setbotdelete <chat_id> <bot_username> <seconds>")
	}
	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id. Используй числовой ID чата.")
	}
	botUsername, err := storage.NormalizeBotUsername(args[1])
	if err != nil {
		return c.Send("Неверный bot_username. Укажи username bot-аккаунта.")
	}
	ttlSec, err := strconv.Atoi(args[2])
	if err != nil || ttlSec < 0 {
		return c.Send("Неверный seconds. Укажи целое число >= 0.")
	}

	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "setbotdelete"); !ok {
		return c.Send(response)
	}

	if err := b.storage.SetBotDeleteRule(chatID, botUsername, ttlSec); err != nil {
		b.logger.Error("set bot delete rule failed", "chat_id", chatID, "bot_username", botUsername, "ttl_sec", ttlSec, "error", err)
		return c.Send("Не удалось сохранить правило автоудаления bot-сообщений.")
	}

	b.logger.Info("bot delete rule updated", "chat_id", chatID, "bot_username", botUsername, "ttl_sec", ttlSec, "by_user", c.Sender().ID)
	if ttlSec == 0 {
		return c.Send(fmt.Sprintf("Автоудаление сообщений от @%s в чате %d выключено.", botUsername, chatID))
	}
	return c.Send(fmt.Sprintf("Автоудаление сообщений от @%s в чате %d: %d сек.", botUsername, chatID, ttlSec))
}

// handleDelBotDelete обрабатывает команду /delbotdelete <chat_id> <bot_username>.
func (b *Bot) handleDelBotDelete(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 {
		return c.Send("Использование: /delbotdelete <chat_id> <bot_username>")
	}
	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id. Используй числовой ID чата.")
	}
	botUsername, err := storage.NormalizeBotUsername(args[1])
	if err != nil {
		return c.Send("Неверный bot_username. Укажи username bot-аккаунта.")
	}

	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "delbotdelete"); !ok {
		return c.Send(response)
	}

	if err := b.storage.DeleteBotDeleteRule(chatID, botUsername); err != nil {
		b.logger.Error("delete bot delete rule failed", "chat_id", chatID, "bot_username", botUsername, "error", err)
		return c.Send("Не удалось удалить правило автоудаления bot-сообщений.")
	}

	b.logger.Info("bot delete rule deleted", "chat_id", chatID, "bot_username", botUsername, "by_user", c.Sender().ID)
	return c.Send(fmt.Sprintf("Правило автоудаления сообщений от @%s в чате %d удалено.", botUsername, chatID))
}

// handleListBotDelete обрабатывает команду /listbotdelete <chat_id>.
func (b *Bot) handleListBotDelete(c tele.Context) error {
	args := c.Args()
	if len(args) < 1 {
		return c.Send("Использование: /listbotdelete <chat_id>")
	}
	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Неверный chat_id. Используй числовой ID чата.")
	}

	if ok, response := b.ensureCommandSenderIsAdmin(c, chatID, "listbotdelete"); !ok {
		return c.Send(response)
	}

	rules, err := b.storage.ListBotDeleteRules(chatID)
	if err != nil {
		b.logger.Error("list bot delete rules failed", "chat_id", chatID, "error", err)
		return c.Send("Не удалось получить правила автоудаления bot-сообщений.")
	}
	if len(rules) == 0 {
		return c.Send(fmt.Sprintf("Для чата %d нет правил автоудаления bot-сообщений.", chatID))
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "Правила автоудаления bot-сообщений для чата %d:", chatID)
	for _, rule := range rules {
		fmt.Fprintf(&builder, "\n- @%s: %d сек.", rule.BotUsername, rule.TTLSec)
	}
	builder.WriteString("\n\nУдалить правило: /delbotdelete <chat_id> <bot_username>")
	return c.Send(builder.String())
}

func (b *Bot) ensureCommandSenderIsAdmin(c tele.Context, chatID int64, action string) (allowed bool, response string) {
	member, err := c.Bot().ChatMemberOf(&tele.Chat{ID: chatID}, c.Sender())
	if err != nil {
		b.logger.Warn("command admin check failed", "action", action, "chat_id", chatID, "user_id", c.Sender().ID, "error", err)
		return false, fmt.Sprintf("Не удалось проверить права. Возможно, я не добавлен в чат %d.", chatID)
	}
	if member.Role != tele.Administrator && member.Role != tele.Creator {
		return false, fmt.Sprintf("Ты не администратор чата %d.", chatID)
	}
	return true, ""
}
