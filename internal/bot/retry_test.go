package bot

import (
	"errors"
	"log/slog"
	"testing"

	tele "gopkg.in/telebot.v3"
)

func TestWithRetry_ImmediateSuccess(t *testing.T) {
	logger := slog.Default()
	calls := 0

	err := withRetry(func() error {
		calls++
		return nil
	}, logger)

	if err != nil {
		t.Errorf("ожидалось nil, получено %v", err)
	}
	if calls != 1 {
		t.Errorf("ожидался 1 вызов, получено %d", calls)
	}
}

func TestWithRetry_NonFloodError(t *testing.T) {
	logger := slog.Default()
	calls := 0
	expectedErr := errors.New("some error")

	err := withRetry(func() error {
		calls++
		return expectedErr
	}, logger)

	if !errors.Is(err, expectedErr) {
		t.Errorf("ожидалась ошибка %v, получено %v", expectedErr, err)
	}
	if calls != 1 {
		t.Errorf("ожидался 1 вызов (без повторов), получено %d", calls)
	}
}

func TestWithRetry_FloodRetry(t *testing.T) {
	logger := slog.Default()
	calls := 0

	err := withRetry(func() error {
		calls++
		if calls == 1 {
			// RetryAfter=0 чтобы тест не ждал
			return tele.FloodError{RetryAfter: 0}
		}
		return nil
	}, logger)

	if err != nil {
		t.Errorf("ожидалось nil, получено %v", err)
	}
	if calls != 2 {
		t.Errorf("ожидалось 2 вызова, получено %d", calls)
	}
}
