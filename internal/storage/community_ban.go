package storage

import (
	"database/sql"
	"errors"
	"time"
)

const (
	CommunityBanStatusOpen         = "open"
	CommunityBanStatusBanned       = "banned"
	CommunityBanStatusClosedManual = "closed_manual"
)

// ModerationCase хранит состояние community-ban кейса.
type ModerationCase struct {
	ID                 int64
	ChatID             int64
	SpamMessageID      int64
	SuspectUserID      int64
	BotReplyMessageID  sql.NullInt64
	LogChatID          int64
	LogReportMessageID sql.NullInt64
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

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

// CreateModerationCase создаёт новый кейс или возвращает существующий.
func (s *Storage) CreateModerationCase(chatID, spamMessageID, suspectUserID, logChatID int64) (*ModerationCase, error) {
	_, err := s.db.Exec(`
		INSERT INTO moderation_case (chat_id, spam_message_id, suspect_user_id, log_chat_id, status)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(chat_id, spam_message_id) DO NOTHING
	`, chatID, spamMessageID, suspectUserID, logChatID, CommunityBanStatusOpen)
	if err != nil {
		return nil, errors.Join(errors.New("не удалось создать moderation case"), err)
	}
	return s.GetModerationCaseByMessage(chatID, spamMessageID)
}

// GetModerationCaseByMessage возвращает кейс по исходному сообщению.
func (s *Storage) GetModerationCaseByMessage(chatID, spamMessageID int64) (*ModerationCase, error) {
	row := s.db.QueryRow(`
		SELECT id, chat_id, spam_message_id, suspect_user_id,
			bot_reply_message_id, log_chat_id, log_report_message_id,
			status, created_at, updated_at
		FROM moderation_case
		WHERE chat_id = ? AND spam_message_id = ?
	`, chatID, spamMessageID)
	return scanModerationCase(row)
}

// GetModerationCase возвращает кейс по ID.
func (s *Storage) GetModerationCase(caseID int64) (*ModerationCase, error) {
	row := s.db.QueryRow(`
		SELECT id, chat_id, spam_message_id, suspect_user_id,
			bot_reply_message_id, log_chat_id, log_report_message_id,
			status, created_at, updated_at
		FROM moderation_case
		WHERE id = ?
	`, caseID)
	return scanModerationCase(row)
}

// SetModerationCaseMessages сохраняет сообщения бота, созданные для кейса.
func (s *Storage) SetModerationCaseMessages(caseID int64, botReplyMessageID, logReportMessageID int) error {
	_, err := s.db.Exec(`
		UPDATE moderation_case
		SET bot_reply_message_id = ?,
			log_report_message_id = ?,
			updated_at = datetime('now')
		WHERE id = ?
	`, botReplyMessageID, logReportMessageID, caseID)
	if err != nil {
		return errors.Join(errors.New("не удалось сохранить сообщения кейса"), err)
	}
	return nil
}

// AddModerationVote регистрирует голос и возвращает новый счётчик голосов.
func (s *Storage) AddModerationVote(caseID, voterUserID int64) (votes int, added bool, err error) {
	result, err := s.db.Exec(`
		INSERT OR IGNORE INTO moderation_vote (case_id, voter_user_id)
		VALUES (?, ?)
	`, caseID, voterUserID)
	if err != nil {
		return 0, false, errors.Join(errors.New("не удалось сохранить голос"), err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, false, errors.Join(errors.New("не удалось получить число добавленных голосов"), err)
	}
	votes, err = s.GetModerationVoteCount(caseID)
	if err != nil {
		return 0, false, err
	}
	return votes, rowsAffected > 0, nil
}

// GetModerationVoteCount возвращает число голосов за кейс.
func (s *Storage) GetModerationVoteCount(caseID int64) (int, error) {
	row := s.db.QueryRow(`SELECT COUNT(*) FROM moderation_vote WHERE case_id = ?`, caseID)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, errors.Join(errors.New("не удалось посчитать голоса"), err)
	}
	return count, nil
}

// CloseModerationCase закрывает кейс без бана.
func (s *Storage) CloseModerationCase(caseID int64) error {
	_, err := s.db.Exec(`
		UPDATE moderation_case
		SET status = ?, updated_at = datetime('now')
		WHERE id = ? AND status = ?
	`, CommunityBanStatusClosedManual, caseID, CommunityBanStatusOpen)
	if err != nil {
		return errors.Join(errors.New("не удалось закрыть moderation case"), err)
	}
	return nil
}

// MarkModerationCaseBanned отмечает кейс как завершённый баном.
func (s *Storage) MarkModerationCaseBanned(caseID int64) error {
	_, err := s.db.Exec(`
		UPDATE moderation_case
		SET status = ?, updated_at = datetime('now')
		WHERE id = ? AND status = ?
	`, CommunityBanStatusBanned, caseID, CommunityBanStatusOpen)
	if err != nil {
		return errors.Join(errors.New("не удалось обновить статус moderation case"), err)
	}
	return nil
}

func scanModerationCase(row *sql.Row) (*ModerationCase, error) {
	var item ModerationCase
	var createdAt string
	var updatedAt string
	if err := row.Scan(
		&item.ID,
		&item.ChatID,
		&item.SpamMessageID,
		&item.SuspectUserID,
		&item.BotReplyMessageID,
		&item.LogChatID,
		&item.LogReportMessageID,
		&item.Status,
		&createdAt,
		&updatedAt,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Join(errors.New("не удалось получить moderation case"), err)
	}

	item.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt) //nolint:errcheck,gosec // фиксированный формат
	item.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt) //nolint:errcheck,gosec // фиксированный формат
	return &item, nil
}
