package bot

import (
	"github.com/datspike/swim-bot/internal/storage"

	tele "gopkg.in/telebot.v3"
)

// detectContext определяет контекст спам-сообщения: спам-волна, реактивный или органический.
// Приоритет: SpamWave > Reactive > Organic.
func (b *Bot) detectContext(chatID, userID int64, msg *tele.Message, cfg *storage.ChatConfig) storage.MessageContext {
	// спам-волна: много срабатываний за короткий период
	spamCount, err := b.storage.SpamCountSince(chatID, cfg.SpamDensityWindowMin)
	if err != nil {
		b.logger.Error("spam count since failed", "chat_id", chatID, "error", err)
	} else if spamCount >= cfg.SpamDensityThreshold {
		return storage.ContextSpamWave
	}

	// реактивный: reply на бота
	if msg != nil && msg.ReplyTo != nil && msg.ReplyTo.Sender != nil &&
		b.bot != nil && b.bot.Me != nil && msg.ReplyTo.Sender.ID == b.bot.Me.ID {
		return storage.ContextReactive
	}

	// реактивный: спам от другого пользователя в окне
	hasRecent, err := b.storage.HasRecentSpamByOther(chatID, userID, cfg.ReactiveWindowMin)
	if err != nil {
		b.logger.Error("has recent spam by other failed", "chat_id", chatID, "error", err)
	} else if hasRecent {
		return storage.ContextReactive
	}

	return storage.ContextOrganic
}

// spamResult содержит результат обработки спам-сообщения.
type spamResult struct {
	Action    storage.SpamAction
	Remaining int    // оставшиеся попытки
	Message   string // текстовое сообщение для чата
}

// processSpam обрабатывает спам-сообщение: обновляет счётчик и определяет действие.
func (b *Bot) processSpam(chatID, userID int64, msg *tele.Message, cfg *storage.ChatConfig) (*spamResult, error) {
	// получаем или создаём счётчик
	counter, err := b.storage.GetOrCreateSpamCounter(chatID, userID, cfg.DailyLimit)
	if err != nil {
		return nil, err
	}

	// кикнутый ранее — мгновенный кик
	if counter.Kicked {
		return &spamResult{Action: storage.ActionKick}, nil
	}

	// определяем контекст
	ctx := b.detectContext(chatID, userID, msg, cfg)

	// при реактивном/спам-волне снижаем лимит если он ещё не снижен
	if ctx != storage.ContextOrganic && counter.EffectiveLimit > cfg.ReactiveLimit {
		newLimit := cfg.ReactiveLimit
		if newLimit < 1 {
			newLimit = 1
		}
		if err := b.storage.UpdateEffectiveLimit(chatID, userID, newLimit); err != nil {
			return nil, err
		}
		counter.EffectiveLimit = newLimit
	}

	// инкрементируем счётчик
	counter, err = b.storage.IncrementSpamCounter(chatID, userID)
	if err != nil {
		return nil, err
	}

	remaining := counter.EffectiveLimit - counter.Count

	// определяем действие на основе текущего состояния
	switch {
	case counter.Count > counter.EffectiveLimit:
		// уже исчерпал попытки ранее (получил "наплавались") — стикер + кик
		return &spamResult{Action: storage.ActionKick}, nil

	case remaining == 0:
		// последняя попытка израсходована — «наплавались»
		return &spamResult{
			Action:    storage.ActionFinalWarning,
			Remaining: 0,
			Message:   "Все, на сегодня наплавались",
		}, nil

	default:
		// ещё есть попытки — предупреждение
		result := &spamResult{
			Action:    storage.ActionWarning,
			Remaining: remaining,
		}

		if ctx != storage.ContextOrganic && counter.Count == 1 {
			// первое сообщение в реактивном контексте — специальное сообщение
			result.Message = "Слишком часто плаваете, ворую попытки"
		} else {
			result.Message = formatRemaining(remaining)
		}

		return result, nil
	}
}

// formatRemaining форматирует сообщение об оставшихся попытках.
func formatRemaining(remaining int) string {
	suffix := "раз"
	if remaining >= 2 && remaining <= 4 {
		suffix = "раза"
	}
	return "Вы можете поплавать ещё " + itoa(remaining) + " " + suffix
}

// itoa конвертирует int в строку (избегаем импорта strconv для одного вызова).
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	return digits
}
