package storage

import (
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"testing"
)

// testLogger создаёт тестовый логгер.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// setupTestDB создаёт in-memory базу данных с миграциями.
func setupTestDB(t *testing.T) *Storage {
	t.Helper()

	store, err := NewStorage(":memory:", testLogger())
	if err != nil {
		t.Fatalf("не удалось создать storage: %v", err)
	}

	if err := Migrate(store.DB(), testLogger()); err != nil {
		t.Fatalf("не удалось выполнить миграции: %v", err)
	}

	t.Cleanup(func() {
		store.Close()
	})

	return store
}

func TestGetChatConfig_NotFound(t *testing.T) {
	store := setupTestDB(t)

	cfg, err := store.GetChatConfig(-123456)
	if err != nil {
		t.Errorf("ожидал nil error, получил: %v", err)
	}
	if cfg != nil {
		t.Errorf("ожидал nil config, получил: %+v", cfg)
	}
}

func TestUpsertTrackedBot(t *testing.T) {
	tests := []struct {
		name       string
		chatID     int64
		trackedBot string
		wantBot    string
	}{
		{
			name:       "simple username",
			chatID:     -100001,
			trackedBot: "mlversebot",
			wantBot:    "mlversebot",
		},
		{
			name:       "username with @",
			chatID:     -100002,
			trackedBot: "@SpamBot",
			wantBot:    "spambot",
		},
		{
			name:       "uppercase username",
			chatID:     -100003,
			trackedBot: "TestBot123",
			wantBot:    "testbot123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupTestDB(t)

			_, err := store.UpsertTrackedBot(tt.chatID, tt.trackedBot)
			if err != nil {
				t.Fatalf("UpsertTrackedBot failed: %v", err)
			}

			cfg, err := store.GetChatConfig(tt.chatID)
			if err != nil {
				t.Fatalf("GetChatConfig failed: %v", err)
			}

			if cfg.TrackedBot != tt.wantBot {
				t.Errorf("TrackedBot = %q, want %q", cfg.TrackedBot, tt.wantBot)
			}

			if cfg.IsActive {
				t.Error("ожидал is_active = false (стикер не задан)")
			}
		})
	}
}

func TestUpsertTrackedBot_Update(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	// первая вставка
	_, err := store.UpsertTrackedBot(chatID, "oldbot")
	if err != nil {
		t.Fatalf("первый UpsertTrackedBot failed: %v", err)
	}

	// обновление
	_, err = store.UpsertTrackedBot(chatID, "newbot")
	if err != nil {
		t.Fatalf("второй UpsertTrackedBot failed: %v", err)
	}

	cfg, err := store.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}

	if cfg.TrackedBot != "newbot" {
		t.Errorf("TrackedBot = %q, want %q", cfg.TrackedBot, "newbot")
	}
}

func TestUpsertStickerFileID_ActivatesBot(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	// сначала задаём tracked_bot
	_, err := store.UpsertTrackedBot(chatID, "spambot")
	if err != nil {
		t.Fatalf("UpsertTrackedBot failed: %v", err)
	}

	// затем задаём стикер — бот должен активироваться
	err = store.UpsertStickerFileID(chatID, "CAACAgIAAxkB123")
	if err != nil {
		t.Fatalf("UpsertStickerFileID failed: %v", err)
	}

	cfg, err := store.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}

	if !cfg.IsActive {
		t.Error("ожидал is_active = true после задания стикера")
	}

	if cfg.StickerFileID != "CAACAgIAAxkB123" {
		t.Errorf("StickerFileID = %q, want %q", cfg.StickerFileID, "CAACAgIAAxkB123")
	}
}

func TestUpsertStickerFileID_WithoutTrackedBot(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	// задаём стикер БЕЗ tracked_bot — бот НЕ должен активироваться
	err := store.UpsertStickerFileID(chatID, "CAACAgIAAxkB123")
	if err != nil {
		t.Fatalf("UpsertStickerFileID failed: %v", err)
	}

	cfg, err := store.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}

	if cfg.IsActive {
		t.Error("ожидал is_active = false (tracked_bot не задан)")
	}
}

func TestInsertActionLog(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	// создаём конфиг (для foreign key)
	_, err := store.UpsertTrackedBot(chatID, "spambot")
	if err != nil {
		t.Fatalf("UpsertTrackedBot failed: %v", err)
	}

	// записываем срабатывание
	err = store.InsertActionLog(chatID, 12345, 100, sql.NullInt64{Int64: 101, Valid: true}, ContextOrganic, ActionSticker)
	if err != nil {
		t.Fatalf("InsertActionLog failed: %v", err)
	}

	// проверяем через GetStats
	stats, err := store.GetStats(chatID)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalCount != 1 {
		t.Errorf("TotalCount = %d, want 1", stats.TotalCount)
	}
}

func TestInsertActionLog_NullReplyMessageID(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	// создаём конфиг
	_, err := store.UpsertTrackedBot(chatID, "spambot")
	if err != nil {
		t.Fatalf("UpsertTrackedBot failed: %v", err)
	}

	// записываем срабатывание с NULL reply_message_id
	err = store.InsertActionLog(chatID, 12345, 100, sql.NullInt64{Valid: false}, ContextOrganic, ActionSticker)
	if err != nil {
		t.Fatalf("InsertActionLog failed: %v", err)
	}
}

func TestChatConfig_NewFields_Defaults(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	_, err := store.UpsertTrackedBot(chatID, "spambot")
	if err != nil {
		t.Fatalf("UpsertTrackedBot failed: %v", err)
	}

	cfg, err := store.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}

	// дефолтные значения из миграции 003
	if cfg.TestMode {
		t.Error("TestMode: ожидалось false по умолчанию")
	}
	if cfg.RingBufferSize != 20 {
		t.Errorf("RingBufferSize: ожидалось 20, получено %d", cfg.RingBufferSize)
	}
	if cfg.RingBufferThreshold != 2 {
		t.Errorf("RingBufferThreshold: ожидалось 2, получено %d", cfg.RingBufferThreshold)
	}
}

func TestGetStats_Empty(t *testing.T) {
	store := setupTestDB(t)

	stats, err := store.GetStats(-123456)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0", stats.TotalCount)
	}

	if stats.TrackedBot != "" {
		t.Errorf("TrackedBot = %q, want empty", stats.TrackedBot)
	}
}

func TestGetStats_WithActions(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	// настраиваем чат
	_, err := store.UpsertTrackedBot(chatID, "spambot")
	if err != nil {
		t.Fatalf("UpsertTrackedBot failed: %v", err)
	}

	err = store.UpsertStickerFileID(chatID, "sticker123")
	if err != nil {
		t.Fatalf("UpsertStickerFileID failed: %v", err)
	}

	// записываем несколько срабатываний
	for i := 0; i < 5; i++ {
		err = store.InsertActionLog(chatID, int64(1000+i), int64(100+i), sql.NullInt64{Int64: int64(200 + i), Valid: true}, ContextOrganic, ActionSticker)
		if err != nil {
			t.Fatalf("InsertActionLog failed: %v", err)
		}
	}

	stats, err := store.GetStats(chatID)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalCount != 5 {
		t.Errorf("TotalCount = %d, want 5", stats.TotalCount)
	}

	if stats.TrackedBot != "spambot" {
		t.Errorf("TrackedBot = %q, want %q", stats.TrackedBot, "spambot")
	}

	if !stats.IsActive {
		t.Error("ожидал IsActive = true")
	}

	if !stats.LastTrigger.Valid {
		t.Error("ожидал LastTrigger.Valid = true")
	}
}

func TestWithSQLiteRetry_Success(t *testing.T) {
	calls := 0
	err := withSQLiteRetry(func() error {
		calls++
		return nil
	}, 3)
	if err != nil {
		t.Errorf("ожидалось nil, получено %v", err)
	}
	if calls != 1 {
		t.Errorf("ожидался 1 вызов, получено %d", calls)
	}
}

func TestWithSQLiteRetry_NonBusyError(t *testing.T) {
	calls := 0
	err := withSQLiteRetry(func() error {
		calls++
		return errors.New("some other error")
	}, 3)
	if err == nil {
		t.Error("ожидалась ошибка")
	}
	if calls != 1 {
		t.Errorf("ожидался 1 вызов (без повторов), получено %d", calls)
	}
}

func TestIsBusyError(t *testing.T) {
	if isBusyError(nil) {
		t.Error("nil не должен быть busy error")
	}
	if !isBusyError(errors.New("SQLITE_BUSY: database is locked")) {
		t.Error("SQLITE_BUSY должен распознаваться")
	}
	if isBusyError(errors.New("some other error")) {
		t.Error("обычная ошибка не должна быть busy error")
	}
}
