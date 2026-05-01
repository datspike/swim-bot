package bot

import (
	"github.com/datspike/swim-bot/internal/storage"
)

// spamResult содержит результат обработки спам-сообщения.
type spamResult struct {
	Action            storage.SpamAction
	Remaining         int    // оставшиеся попытки
	Count             int    // текущий счётчик (сколько раз сработало)
	Limit             int    // лимит попыток
	RBSpamCount       int    // спам-записей от других в ring buffer
	RBThreshold       int    // порог для reactive
	Message           string // текстовое сообщение для чата
	AlreadyRestricted bool   // пользователь уже ограничен за текущие сутки
}

// detectContext определяет контекст спам-сообщения по ring buffer.
// Reactive если N+ спам-записей от других пользователей в окне, иначе Organic.
func (b *Bot) detectContext(chatID, userID int64, cfg *storage.ChatConfig) storage.MessageContext {
	if cfg.RingBufferThreshold <= 0 {
		return storage.ContextOrganic
	}

	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	spamCount := rb.SpamCountByOthers(userID)
	if spamCount >= cfg.RingBufferThreshold {
		return storage.ContextReactive
	}
	return storage.ContextOrganic
}

// processSpam обрабатывает спам по логике ring buffer + restrict.
// Consume 1 + steal 1 в реактивном контексте, restrict вместо кика.
func (b *Bot) processSpam(chatID, userID int64, cfg *storage.ChatConfig) (*spamResult, error) {
	// получаем или создаём счётчик
	counter, err := b.storage.GetOrCreateSpamCounter(chatID, userID, cfg.DailyLimit)
	if err != nil {
		return nil, err
	}

	// уже ограничен ранее — пропускаем повторный restrict
	if counter.Kicked {
		return &spamResult{Count: counter.Count, Limit: counter.EffectiveLimit, AlreadyRestricted: true}, nil
	}

	// определяем контекст + получаем rb stats
	ctx := b.detectContext(chatID, userID, cfg)
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rbSpamCount := rb.SpamCountByOthers(userID)
	rbThreshold := cfg.RingBufferThreshold

	// consume 1 попытку
	counter, err = b.storage.IncrementSpamCounter(chatID, userID)
	if err != nil {
		return nil, err
	}

	remaining := counter.EffectiveLimit - counter.Count

	// реактивный контекст: штраф
	if ctx == storage.ContextReactive && remaining > 0 {
		// воруем ещё 1 попытку
		counter, err = b.storage.IncrementSpamCounter(chatID, userID)
		if err != nil {
			return nil, err
		}
		remaining = counter.EffectiveLimit - counter.Count

		if remaining <= 0 {
			// штраф + лимит исчерпан
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

		// штраф, ещё есть попытки
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
		// лимит исчерпан -> restrict
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
