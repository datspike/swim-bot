package storage

import (
	"errors"
	"strings"
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
	}, 3)
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

// UpdateEffectiveLimit устанавливает новый эффективный лимит (при переходе в reactive/spam_wave).
// Лимит не может быть ниже 1 (минимум 1 попытка в день).
func (s *Storage) UpdateEffectiveLimit(chatID, userID int64, newLimit int) error {
	if newLimit < 1 {
		newLimit = 1
	}

	_, err := s.db.Exec(`
		UPDATE spam_counter
		SET effective_limit = ?, updated_at = datetime('now')
		WHERE chat_id = ? AND user_id = ? AND date = date('now')
	`, newLimit, chatID, userID)
	if err != nil {
		return errors.Join(errors.New("не удалось обновить effective_limit"), err)
	}

	return nil
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
	}, 3)
	if err != nil {
		return errors.Join(errors.New("не удалось пометить kicked"), err)
	}

	return nil
}

// SpamCountSince возвращает количество спам-срабатываний в чате за последние N минут.
func (s *Storage) SpamCountSince(chatID int64, windowMinutes int) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM action_log
		WHERE chat_id = ? AND created_at >= datetime('now', ? || ' minutes')
	`, chatID, -windowMinutes).Scan(&count)
	if err != nil {
		return 0, errors.Join(errors.New("не удалось подсчитать спам за период"), err)
	}

	return count, nil
}

// HasRecentSpamByOther проверяет наличие спама от другого пользователя в чате за окно реактивности.
func (s *Storage) HasRecentSpamByOther(chatID, currentUserID int64, windowMinutes int) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM action_log
			WHERE chat_id = ? AND user_id != ?
				AND created_at >= datetime('now', ? || ' minutes')
		)
	`, chatID, currentUserID, -windowMinutes).Scan(&exists)
	if err != nil {
		return false, errors.Join(errors.New("не удалось проверить спам от других"), err)
	}

	return exists, nil
}

// UpsertTrackedStickerPack сохраняет имя стикерпака для чата (FR-005).
// При повторной настройке — замена.
func (s *Storage) UpsertTrackedStickerPack(chatID int64, packName string) error {
	packName = strings.ToLower(packName)

	_, err := s.db.Exec(`
		INSERT INTO chat_config (chat_id, tracked_sticker_pack, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(chat_id) DO UPDATE SET
			tracked_sticker_pack = excluded.tracked_sticker_pack,
			updated_at = datetime('now')
	`, chatID, packName)
	if err != nil {
		return errors.Join(errors.New("не удалось сохранить tracked_sticker_pack"), err)
	}

	return nil
}

// SetTestMode включает/выключает тестовый режим для чата (FR-014, FR-020).
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

// IncrementSpamCounterBy увеличивает счётчик спама на указанное количество (FR-007).
// Используется для штрафа за групповой спам.
func (s *Storage) IncrementSpamCounterBy(chatID, userID int64, amount int) (*SpamCounter, error) {
	_, err := s.db.Exec(`
		INSERT INTO spam_counter (chat_id, user_id, date, count, effective_limit)
		VALUES (?, ?, date('now'), ?, 4)
		ON CONFLICT(chat_id, user_id, date) DO UPDATE SET
			count = count + ?, updated_at = datetime('now')
	`, chatID, userID, amount, amount)
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
	affected, _ := result.RowsAffected()
	return affected, nil
}

// DeleteSticker удаляет стикер из конфигурации чата (FR-006).
func (s *Storage) DeleteSticker(chatID int64) error {
	_, err := s.db.Exec(`
		UPDATE chat_config
		SET sticker_file_id = '', updated_at = datetime('now')
		WHERE chat_id = ?
	`, chatID)
	if err != nil {
		return errors.Join(errors.New("не удалось удалить стикер"), err)
	}
	return nil
}

// UpdateRateLimitConfig обновляет настройки rate limiting для чата, включая ring buffer (FR-010b).
func (s *Storage) UpdateRateLimitConfig(chatID int64, dailyLimit, reactiveLimit, reactiveWindowMin, spamDensityThreshold, spamDensityWindowMin, ringBufferSize, ringBufferThreshold int) error {
	_, err := s.db.Exec(`
		UPDATE chat_config
		SET daily_limit = ?, reactive_limit = ?, reactive_window_min = ?,
			spam_density_threshold = ?, spam_density_window_min = ?,
			ring_buffer_size = ?, ring_buffer_threshold = ?,
			updated_at = datetime('now')
		WHERE chat_id = ?
	`, dailyLimit, reactiveLimit, reactiveWindowMin, spamDensityThreshold, spamDensityWindowMin, ringBufferSize, ringBufferThreshold, chatID)
	if err != nil {
		return errors.Join(errors.New("не удалось обновить настройки rate limiting"), err)
	}

	return nil
}
