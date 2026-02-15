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
	Action      storage.SpamAction
	Remaining   int    // оставшиеся попытки
	Count       int    // текущий счётчик (сколько раз сработало)
	Limit       int    // лимит попыток
	RBSpamCount int    // спам-записей от других в ring buffer
	RBThreshold int    // порог для reactive
	Message     string // текстовое сообщение для чата
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

// detectContextNew определяет контекст спам-сообщения по ring buffer (FR-010a).
// Reactive если N+ спам-записей от других пользователей в окне, иначе Organic.
func (b *Bot) detectContextNew(chatID, userID int64, cfg *storage.ChatConfig) storage.MessageContext {
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	spamCount := rb.SpamCountByOthers(userID)
	if spamCount >= cfg.RingBufferThreshold {
		return storage.ContextReactive
	}
	return storage.ContextOrganic
}

// processSpamNew обрабатывает спам по новой логике (FR-007..FR-009).
// Новая логика: consume 1 + steal 1 в реактивном контексте, restrict вместо кика.
func (b *Bot) processSpamNew(chatID, userID int64, msg *tele.Message, cfg *storage.ChatConfig) (*spamResult, error) {
	// получаем или создаём счётчик
	counter, err := b.storage.GetOrCreateSpamCounter(chatID, userID, cfg.DailyLimit)
	if err != nil {
		return nil, err
	}

	// уже ограничен ранее — пропускаем (не нужен повторный restrict)
	if counter.Kicked {
		return &spamResult{Action: storage.ActionRestrict, Count: counter.Count, Limit: counter.EffectiveLimit}, nil
	}

	// определяем контекст + получаем rb stats
	ctx := b.detectContextNew(chatID, userID, cfg)
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rbSpamCount := rb.SpamCountByOthers(userID)
	rbThreshold := cfg.RingBufferThreshold

	// consume 1 попытку
	counter, err = b.storage.IncrementSpamCounter(chatID, userID)
	if err != nil {
		return nil, err
	}

	remaining := counter.EffectiveLimit - counter.Count

	// реактивный контекст: штраф (FR-007, FR-008, FR-009)
	if ctx == storage.ContextReactive && remaining > 0 {
		// воруем ещё 1 попытку
		counter, err = b.storage.IncrementSpamCounter(chatID, userID)
		if err != nil {
			return nil, err
		}
		remaining = counter.EffectiveLimit - counter.Count

		if remaining <= 0 {
			// FR-008: штраф + лимит исчерпан
			return &spamResult{
				Action:      storage.ActionRestrict,
				Remaining:   0,
				Count:       counter.Count,
				Limit:       counter.EffectiveLimit,
				RBSpamCount: rbSpamCount,
				RBThreshold: rbThreshold,
				Message:     "Группами плавать нежелательно, ворую попытку. Все, на сегодня наплавались",
			}, nil
		}

		// FR-007: штраф, ещё есть попытки
		return &spamResult{
			Action:      storage.ActionWarning,
			Remaining:   remaining,
			Count:       counter.Count,
			Limit:       counter.EffectiveLimit,
			RBSpamCount: rbSpamCount,
			RBThreshold: rbThreshold,
			Message:     "Группами плавать нежелательно, ворую попытку, осталось: " + itoa(remaining),
		}, nil
	}

	// определяем действие
	switch {
	case remaining <= 0:
		// FR-009 / FR-004: лимит исчерпан -> restrict
		return &spamResult{
			Action:      storage.ActionRestrict,
			Remaining:   0,
			Count:       counter.Count,
			Limit:       counter.EffectiveLimit,
			RBSpamCount: rbSpamCount,
			RBThreshold: rbThreshold,
			Message:     "Все, на сегодня наплавались",
		}, nil

	default:
		// ещё есть попытки — предупреждение
		return &spamResult{
			Action:      storage.ActionWarning,
			Remaining:   remaining,
			Count:       counter.Count,
			Limit:       counter.EffectiveLimit,
			RBSpamCount: rbSpamCount,
			RBThreshold: rbThreshold,
			Message:     formatRemaining(remaining),
		}, nil
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
