package storage

import "errors"

// EnsureChatConfig создаёт запись чата с дефолтами, если её ещё нет.
func (s *Storage) EnsureChatConfig(chatID int64) error {
	_, err := s.db.Exec(`
		INSERT INTO chat_config (chat_id, updated_at)
		VALUES (?, datetime('now'))
		ON CONFLICT(chat_id) DO UPDATE SET updated_at = datetime('now')
	`, chatID)
	if err != nil {
		return errors.Join(errors.New("не удалось подготовить конфиг чата"), err)
	}
	return nil
}

// SetCommunityBanEnabled включает или выключает community-ban детекцию для чата.
func (s *Storage) SetCommunityBanEnabled(chatID int64, enabled bool) error {
	if err := s.EnsureChatConfig(chatID); err != nil {
		return err
	}

	_, err := s.db.Exec(`
		UPDATE chat_config
		SET community_ban_enabled = ?,
			is_active = CASE WHEN tracked_bot != '' OR ? THEN 1 ELSE 0 END,
			updated_at = datetime('now')
		WHERE chat_id = ?
	`, enabled, enabled, chatID)
	if err != nil {
		return errors.Join(errors.New("не удалось обновить community ban"), err)
	}
	return nil
}

// SetSpamLogChatID настраивает чат для репортинга community-ban кейсов.
func (s *Storage) SetSpamLogChatID(chatID, targetChatID int64) error {
	if err := s.EnsureChatConfig(chatID); err != nil {
		return err
	}

	_, err := s.db.Exec(`
		UPDATE chat_config
		SET spam_log_chat_id = ?, updated_at = datetime('now')
		WHERE chat_id = ?
	`, targetChatID, chatID)
	if err != nil {
		return errors.Join(errors.New("не удалось обновить spam log chat"), err)
	}
	return nil
}

// SetSpamDeleteTTL настраивает per-chat TTL автоудаления спам-сообщений (0 = выключено).
func (s *Storage) SetSpamDeleteTTL(chatID int64, ttlSec int) error {
	if err := s.EnsureChatConfig(chatID); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		UPDATE chat_config
		SET spam_delete_ttl_sec = ?, updated_at = datetime('now')
		WHERE chat_id = ?
	`, ttlSec, chatID)
	if err != nil {
		return errors.Join(errors.New("не удалось обновить ttl автоудаления спама"), err)
	}
	return nil
}
