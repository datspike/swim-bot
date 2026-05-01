package storage

import (
	"errors"
)

// GetOrCreateSpamCounter возвращает счётчик спама пользователя за текущие сутки (UTC).
// Создаёт запись если не существует, используя defaultLimit как начальный эффективный лимит.
func (s *Storage) GetOrCreateSpamCounter(chatID, userID int64, defaultLimit int) (*SpamCounter, error) {
	_, err := s.db.Exec(`
		INSERT INTO spam_counter (chat_id, user_id, date, count, effective_limit)
		VALUES (?, ?, date('now'), 0, ?)
		ON CONFLICT(chat_id, user_id, date) DO NOTHING
	`, chatID, userID, defaultLimit)
	if err != nil {
		return nil, errors.Join(errors.New("не удалось создать spam_counter"), err)
	}

	row := s.db.QueryRow(`
		SELECT chat_id, user_id, date, count, effective_limit, kicked
		FROM spam_counter
		WHERE chat_id = ? AND user_id = ? AND date = date('now')
	`, chatID, userID)

	var sc SpamCounter
	err = row.Scan(&sc.ChatID, &sc.UserID, &sc.Date, &sc.Count, &sc.EffectiveLimit, &sc.Kicked)
	if err != nil {
		return nil, errors.Join(errors.New("не удалось получить spam_counter"), err)
	}

	return &sc, nil
}

// IncrementSpamCounter увеличивает счётчик спама и возвращает обновлённое значение (FR-013: retry при BUSY).
func (s *Storage) IncrementSpamCounter(chatID, userID int64) (*SpamCounter, error) {
	err := withSQLiteRetry(func() error {
		_, e := s.db.Exec(`
			UPDATE spam_counter
			SET count = count + 1, updated_at = datetime('now')
			WHERE chat_id = ? AND user_id = ? AND date = date('now')
		`, chatID, userID)
		return e
	})
	if err != nil {
		return nil, errors.Join(errors.New("не удалось увеличить spam_counter"), err)
	}

	row := s.db.QueryRow(`
		SELECT chat_id, user_id, date, count, effective_limit, kicked
		FROM spam_counter
		WHERE chat_id = ? AND user_id = ? AND date = date('now')
	`, chatID, userID)

	var sc SpamCounter
	err = row.Scan(&sc.ChatID, &sc.UserID, &sc.Date, &sc.Count, &sc.EffectiveLimit, &sc.Kicked)
	if err != nil {
		return nil, errors.Join(errors.New("не удалось получить обновлённый spam_counter"), err)
	}

	return &sc, nil
}

// MarkKicked помечает пользователя как кикнутого за текущие сутки (FR-013: retry при BUSY).
func (s *Storage) MarkKicked(chatID, userID int64) error {
	err := withSQLiteRetry(func() error {
		_, e := s.db.Exec(`
			UPDATE spam_counter
			SET kicked = 1, updated_at = datetime('now')
			WHERE chat_id = ? AND user_id = ? AND date = date('now')
		`, chatID, userID)
		return e
	})
	if err != nil {
		return errors.Join(errors.New("не удалось пометить kicked"), err)
	}

	return nil
}

// SetTestMode включает/выключает тестовый режим для чата.
func (s *Storage) SetTestMode(chatID int64, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	_, err := s.db.Exec(`
		UPDATE chat_config
		SET test_mode = ?, updated_at = datetime('now')
		WHERE chat_id = ?
	`, val, chatID)
	if err != nil {
		return errors.Join(errors.New("не удалось обновить test_mode"), err)
	}
	return nil
}

// ResetSpamCounters сбрасывает все счётчики спама для чата за сегодня.
// Используется в тестовом режиме для повторного тестирования.
func (s *Storage) ResetSpamCounters(chatID int64) (int64, error) {
	result, err := s.db.Exec(`
		DELETE FROM spam_counter
		WHERE chat_id = ? AND date = date('now')
	`, chatID)
	if err != nil {
		return 0, errors.Join(errors.New("не удалось сбросить счётчики"), err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Join(errors.New("не удалось получить число сброшенных счётчиков"), err)
	}
	return affected, nil
}

// UpdateRateLimitConfig обновляет настройки rate limiting для чата.
// Принимает 3 параметра: dailyLimit, ringBufferSize, ringBufferThreshold.
func (s *Storage) UpdateRateLimitConfig(chatID int64, dailyLimit, ringBufferSize, ringBufferThreshold int) error {
	_, err := s.db.Exec(`
		UPDATE chat_config
		SET daily_limit = ?, ring_buffer_size = ?, ring_buffer_threshold = ?,
			updated_at = datetime('now')
		WHERE chat_id = ?
	`, dailyLimit, ringBufferSize, ringBufferThreshold, chatID)
	if err != nil {
		return errors.Join(errors.New("не удалось обновить настройки rate limiting"), err)
	}

	return nil
}
