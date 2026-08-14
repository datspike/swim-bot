package bot

import (
	"errors"
	"log/slog"
	"time"

	tele "gopkg.in/telebot.v4"
)

// withRetry повторяет fn при получении tele.FloodError, ожидая RetryAfter секунд.
// При других ошибках возвращает сразу.
func withRetry(fn func() error, logger *slog.Logger) error {
	for {
		err := fn()
		if err == nil {
			return nil
		}
		var flood tele.FloodError
		if errors.As(err, &flood) {
			wait := time.Duration(flood.RetryAfter) * time.Second
			logger.Warn("Telegram FloodError, ожидание",
				"retry_after_sec", flood.RetryAfter,
			)
			time.Sleep(wait)
			continue
		}
		return err
	}
}
