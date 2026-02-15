// Package storage отвечает за хранение данных в SQLite.
package storage

import (
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	_ "modernc.org/sqlite" // драйвер SQLite
)

// ChatConfig содержит конфигурацию бота для конкретного чата.
type ChatConfig struct {
	ChatID               int64
	TrackedBot           string
	StickerFileID        string
	IsActive             bool
	TrackedStickerPack   string
	DailyLimit           int
	ReactiveLimit        int
	ReactiveWindowMin    int
	SpamDensityThreshold int
	SpamDensityWindowMin int
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// MessageContext определяет контекст спам-сообщения.
type MessageContext int

const (
	ContextOrganic  MessageContext = 0 // органическое
	ContextReactive MessageContext = 1 // реактивное
	ContextSpamWave MessageContext = 2 // спам-волна
)

// SpamAction определяет действие бота при спаме.
type SpamAction int

const (
	ActionSticker      SpamAction = 0 // стикер (старое поведение)
	ActionWarning      SpamAction = 1 // предупреждение с оставшимися попытками
	ActionFinalWarning SpamAction = 2 // «наплавались» (0 попыток)
	ActionKick         SpamAction = 3 // стикер + кик
)

// SpamCounter содержит состояние счётчика спама пользователя в чате за день.
type SpamCounter struct {
	ChatID         int64
	UserID         int64
	Date           string // YYYY-MM-DD
	Count          int
	EffectiveLimit int
	Kicked         bool
}

// ActionLog содержит запись о срабатывании бота.
type ActionLog struct {
	ID             int64
	ChatID         int64
	UserID         int64
	SpamMessageID  int64
	ReplyMessageID sql.NullInt64
	CreatedAt      time.Time
}

// Stats содержит статистику по чату.
type Stats struct {
	TotalCount  int
	LastTrigger sql.NullTime
	TrackedBot  string
	IsActive    bool
}

// Storage предоставляет методы для работы с базой данных.
type Storage struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewStorage создаёт новое подключение к SQLite с оптимальными настройками.
// DSN должен содержать путь к файлу БД или ":memory:" для in-memory.
func NewStorage(dsn string, logger *slog.Logger) (*Storage, error) {
	// добавляем параметры подключения если их нет
	if !strings.Contains(dsn, "?") {
		dsn += "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, errors.Join(errors.New("не удалось открыть базу данных"), err)
	}

	// проверяем подключение
	if err := db.Ping(); err != nil {
		return nil, errors.Join(errors.New("не удалось подключиться к базе данных"), err)
	}

	return &Storage{db: db, logger: logger}, nil
}

// DB возвращает внутреннее подключение к базе данных (для миграций).
func (s *Storage) DB() *sql.DB {
	return s.db
}

// Close закрывает подключение к базе данных.
func (s *Storage) Close() error {
	return s.db.Close()
}

// GetChatConfig возвращает конфигурацию чата по ID.
// Возвращает nil, nil если конфигурация не найдена.
func (s *Storage) GetChatConfig(chatID int64) (*ChatConfig, error) {
	row := s.db.QueryRow(`
		SELECT chat_id, tracked_bot, sticker_file_id, is_active,
			tracked_sticker_pack, daily_limit, reactive_limit,
			reactive_window_min, spam_density_threshold, spam_density_window_min,
			created_at, updated_at
		FROM chat_config
		WHERE chat_id = ?
	`, chatID)

	var cfg ChatConfig
	var createdAt, updatedAt string
	err := row.Scan(
		&cfg.ChatID, &cfg.TrackedBot, &cfg.StickerFileID, &cfg.IsActive,
		&cfg.TrackedStickerPack, &cfg.DailyLimit, &cfg.ReactiveLimit,
		&cfg.ReactiveWindowMin, &cfg.SpamDensityThreshold, &cfg.SpamDensityWindowMin,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Join(errors.New("не удалось получить конфиг чата"), err)
	}

	// SQLite datetime имеет фиксированный формат, парсинг не должен падать
	cfg.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt) //nolint:errcheck,gosec // фиксированный формат
	cfg.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt) //nolint:errcheck,gosec // фиксированный формат

	return &cfg, nil
}

// UpsertTrackedBot создаёт или обновляет tracked_bot для чата.
// Возвращает true если это новая запись.
func (s *Storage) UpsertTrackedBot(chatID int64, trackedBot string) (bool, error) {
	// нормализация: убираем @ и приводим к lowercase
	trackedBot = strings.TrimPrefix(strings.ToLower(trackedBot), "@")

	result, err := s.db.Exec(`
		INSERT INTO chat_config (chat_id, tracked_bot, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(chat_id) DO UPDATE SET
			tracked_bot = excluded.tracked_bot,
			updated_at = datetime('now')
	`, chatID, trackedBot)
	if err != nil {
		return false, errors.Join(errors.New("не удалось сохранить tracked_bot"), err)
	}

	rowsAffected, _ := result.RowsAffected() //nolint:errcheck,gosec // SQLite драйвер всегда возвращает nil
	return rowsAffected > 0, nil
}

// UpsertStickerFileID создаёт или обновляет sticker_file_id для чата.
// Активирует бота (is_active=1) если tracked_bot уже задан.
func (s *Storage) UpsertStickerFileID(chatID int64, stickerFileID string) error {
	_, err := s.db.Exec(`
		INSERT INTO chat_config (chat_id, sticker_file_id, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(chat_id) DO UPDATE SET
			sticker_file_id = excluded.sticker_file_id,
			is_active = CASE WHEN tracked_bot != '' THEN 1 ELSE 0 END,
			updated_at = datetime('now')
	`, chatID, stickerFileID)
	if err != nil {
		return errors.Join(errors.New("не удалось сохранить sticker_file_id"), err)
	}
	return nil
}

// InsertActionLog записывает срабатывание бота в лог.
func (s *Storage) InsertActionLog(chatID, userID, spamMessageID int64, replyMessageID sql.NullInt64, ctx MessageContext, action SpamAction) error {
	_, err := s.db.Exec(`
		INSERT INTO action_log (chat_id, user_id, spam_message_id, reply_message_id, context, action)
		VALUES (?, ?, ?, ?, ?, ?)
	`, chatID, userID, spamMessageID, replyMessageID, ctx, action)
	if err != nil {
		return errors.Join(errors.New("не удалось записать в action_log"), err)
	}
	return nil
}

// GetStats возвращает статистику по чату.
func (s *Storage) GetStats(chatID int64) (*Stats, error) {
	// получаем конфиг
	cfg, err := s.GetChatConfig(chatID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &Stats{}, nil
	}

	// получаем статистику
	var stats Stats
	stats.TrackedBot = cfg.TrackedBot
	stats.IsActive = cfg.IsActive

	row := s.db.QueryRow(`
		SELECT COUNT(*), MAX(created_at)
		FROM action_log
		WHERE chat_id = ?
	`, chatID)

	var lastTrigger sql.NullString
	err = row.Scan(&stats.TotalCount, &lastTrigger)
	if err != nil {
		return nil, errors.Join(errors.New("не удалось получить статистику"), err)
	}

	if lastTrigger.Valid {
		t, _ := time.Parse("2006-01-02 15:04:05", lastTrigger.String) //nolint:errcheck,gosec // фиксированный формат
		stats.LastTrigger = sql.NullTime{Time: t, Valid: true}
	}

	return &stats, nil
}
