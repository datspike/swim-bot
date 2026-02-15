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

// IncrementSpamCounter увеличивает счётчик спама и возвращает обновлённое значение.
func (s *Storage) IncrementSpamCounter(chatID, userID int64) (*SpamCounter, error) {
	_, err := s.db.Exec(`
		UPDATE spam_counter
		SET count = count + 1, updated_at = datetime('now')
		WHERE chat_id = ? AND user_id = ? AND date = date('now')
	`, chatID, userID)
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

// MarkKicked помечает пользователя как кикнутого за текущие сутки.
func (s *Storage) MarkKicked(chatID, userID int64) error {
	_, err := s.db.Exec(`
		UPDATE spam_counter
		SET kicked = 1, updated_at = datetime('now')
		WHERE chat_id = ? AND user_id = ? AND date = date('now')
	`, chatID, userID)
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

// UpdateRateLimitConfig обновляет настройки rate limiting для чата.
func (s *Storage) UpdateRateLimitConfig(chatID int64, dailyLimit, reactiveLimit, reactiveWindowMin, spamDensityThreshold, spamDensityWindowMin int) error {
	_, err := s.db.Exec(`
		UPDATE chat_config
		SET daily_limit = ?, reactive_limit = ?, reactive_window_min = ?,
			spam_density_threshold = ?, spam_density_window_min = ?,
			updated_at = datetime('now')
		WHERE chat_id = ?
	`, dailyLimit, reactiveLimit, reactiveWindowMin, spamDensityThreshold, spamDensityWindowMin, chatID)
	if err != nil {
		return errors.Join(errors.New("не удалось обновить настройки rate limiting"), err)
	}

	return nil
}
