package storage

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// BotDeleteRule хранит правило автоудаления сообщений от bot-аккаунта.
type BotDeleteRule struct {
	ChatID      int64
	BotUsername string
	TTLSec      int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NormalizeBotUsername нормализует username bot-аккаунта для правил автоудаления.
func NormalizeBotUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	username = strings.TrimPrefix(username, "@")
	username = strings.ToLower(username)
	if username == "" {
		return "", errors.New("username bot-аккаунта не должен быть пустым")
	}
	return username, nil
}

// SetBotDeleteRule создаёт или обновляет правило автоудаления сообщений от bot-аккаунта.
func (s *Storage) SetBotDeleteRule(chatID int64, username string, ttlSec int) error {
	if ttlSec < 0 {
		return errors.New("ttl автоудаления должен быть не меньше 0")
	}
	if ttlSec == 0 {
		return s.DeleteBotDeleteRule(chatID, username)
	}

	botUsername, err := NormalizeBotUsername(username)
	if err != nil {
		return err
	}
	ensureErr := s.EnsureChatConfig(chatID)
	if ensureErr != nil {
		return ensureErr
	}

	_, err = s.db.Exec(`
		INSERT INTO bot_delete_rule (chat_id, bot_username, ttl_sec, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(chat_id, bot_username) DO UPDATE SET
			ttl_sec = excluded.ttl_sec,
			updated_at = datetime('now')
	`, chatID, botUsername, ttlSec)
	if err != nil {
		return errors.Join(errors.New("не удалось сохранить правило автоудаления bot-сообщений"), err)
	}

	_, err = s.db.Exec(`
		UPDATE chat_config
		SET is_active = 1, updated_at = datetime('now')
		WHERE chat_id = ?
	`, chatID)
	if err != nil {
		return errors.Join(errors.New("не удалось активировать чат для автоудаления bot-сообщений"), err)
	}
	return nil
}

// DeleteBotDeleteRule удаляет правило автоудаления сообщений от bot-аккаунта.
func (s *Storage) DeleteBotDeleteRule(chatID int64, username string) error {
	botUsername, err := NormalizeBotUsername(username)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		DELETE FROM bot_delete_rule
		WHERE chat_id = ? AND bot_username = ?
	`, chatID, botUsername)
	if err != nil {
		return errors.Join(errors.New("не удалось удалить правило автоудаления bot-сообщений"), err)
	}
	return nil
}

// GetBotDeleteRule возвращает правило автоудаления сообщений от bot-аккаунта.
func (s *Storage) GetBotDeleteRule(chatID int64, username string) (*BotDeleteRule, error) {
	botUsername, err := NormalizeBotUsername(username)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(`
		SELECT chat_id, bot_username, ttl_sec, created_at, updated_at
		FROM bot_delete_rule
		WHERE chat_id = ? AND bot_username = ?
	`, chatID, botUsername)
	return scanBotDeleteRule(row)
}

// ListBotDeleteRules возвращает правила автоудаления bot-сообщений для чата.
func (s *Storage) ListBotDeleteRules(chatID int64) ([]BotDeleteRule, error) {
	rows, err := s.db.Query(`
		SELECT chat_id, bot_username, ttl_sec, created_at, updated_at
		FROM bot_delete_rule
		WHERE chat_id = ?
		ORDER BY bot_username
	`, chatID)
	if err != nil {
		return nil, errors.Join(errors.New("не удалось получить правила автоудаления bot-сообщений"), err)
	}
	defer rows.Close()

	rules := make([]BotDeleteRule, 0)
	for rows.Next() {
		rule, scanErr := scanBotDeleteRule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		rules = append(rules, *rule)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Join(errors.New("не удалось прочитать правила автоудаления bot-сообщений"), err)
	}
	return rules, nil
}

type botDeleteRuleScanner interface {
	Scan(dest ...any) error
}

func scanBotDeleteRule(scanner botDeleteRuleScanner) (*BotDeleteRule, error) {
	var rule BotDeleteRule
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&rule.ChatID,
		&rule.BotUsername,
		&rule.TTLSec,
		&createdAt,
		&updatedAt,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Join(errors.New("не удалось получить правило автоудаления bot-сообщений"), err)
	}

	rule.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt) //nolint:errcheck,gosec // фиксированный формат
	rule.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt) //nolint:errcheck,gosec // фиксированный формат
	return &rule, nil
}
