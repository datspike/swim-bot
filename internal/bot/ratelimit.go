package bot

import (
	"github.com/datspike/swim-bot/internal/storage"
)

// spamResult содержит результат обработки спам-сообщения.
type spamResult struct {
	Action            storage.SpamAction
	Context           storage.MessageContext
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

	// определяем контекст + получаем rb stats
	ctx := b.detectContext(chatID, userID, cfg)
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rbSpamCount := rb.SpamCountByOthers(userID)
	rbThreshold := cfg.RingBufferThreshold

	// уже ограничен ранее — пропускаем повторный restrict
	if counter.Kicked {
		return &spamResult{
			Context:           ctx,
			Count:             counter.Count,
			Limit:             counter.EffectiveLimit,
			RBSpamCount:       rbSpamCount,
			RBThreshold:       rbThreshold,
			AlreadyRestricted: true,
		}, nil
	}

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
		return buildReactiveSpamResult(counter, ctx, rbSpamCount, rbThreshold), nil
	}

	return buildOrganicSpamResult(counter, ctx, rbSpamCount, rbThreshold), nil
}

// buildOrganicSpamResult формирует результат обычного расхода попытки.
func buildOrganicSpamResult(counter *storage.SpamCounter, ctx storage.MessageContext, rbSpamCount, rbThreshold int) *spamResult {
	remaining := counter.EffectiveLimit - counter.Count
	if remaining <= 0 {
		return newSpamResult(
			storage.ActionRestrict,
			ctx,
			0,
			counter,
			rbSpamCount,
			rbThreshold,
			"Все, на сегодня наплавались",
		)
	}

	return newSpamResult(
		storage.ActionWarning,
		ctx,
		remaining,
		counter,
		rbSpamCount,
		rbThreshold,
		formatRemaining(remaining),
	)
}

// buildReactiveSpamResult формирует результат расхода попытки с reactive-штрафом.
func buildReactiveSpamResult(counter *storage.SpamCounter, ctx storage.MessageContext, rbSpamCount, rbThreshold int) *spamResult {
	remaining := counter.EffectiveLimit - counter.Count
	if remaining <= 0 {
		return newSpamResult(
			storage.ActionRestrict,
			ctx,
			0,
			counter,
			rbSpamCount,
			rbThreshold,
			"Группами плавать нежелательно, ворую попытку. Все, на сегодня наплавались",
		)
	}

	return newSpamResult(
		storage.ActionWarning,
		ctx,
		remaining,
		counter,
		rbSpamCount,
		rbThreshold,
		"Группами плавать нежелательно, ворую попытку, осталось: "+itoa(remaining),
	)
}

// newSpamResult собирает общий результат обработки спама.
func newSpamResult(action storage.SpamAction, ctx storage.MessageContext, remaining int, counter *storage.SpamCounter, rbSpamCount, rbThreshold int, message string) *spamResult {
	return &spamResult{
		Action:      action,
		Context:     ctx,
		Remaining:   remaining,
		Count:       counter.Count,
		Limit:       counter.EffectiveLimit,
		RBSpamCount: rbSpamCount,
		RBThreshold: rbThreshold,
		Message:     message,
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
