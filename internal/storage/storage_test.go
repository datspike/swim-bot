package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
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

func TestWithDefaultPragmas(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want []string
	}{
		{
			name: "plain path",
			dsn:  "test.db",
			want: []string{
				"test.db?",
				"_pragma=busy_timeout(5000)",
				"_pragma=foreign_keys(ON)",
				"_pragma=journal_mode(WAL)",
			},
		},
		{
			name: "existing query",
			dsn:  "file:test.db?cache=shared",
			want: []string{
				"file:test.db?cache=shared&",
				"_pragma=busy_timeout(5000)",
				"_pragma=foreign_keys(ON)",
				"_pragma=journal_mode(WAL)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withDefaultPragmas(tt.dsn)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("withDefaultPragmas(%q) = %q, want substring %q", tt.dsn, got, want)
				}
			}
		})
	}
}

func TestMigrateRejectsFutureSchemaVersion(t *testing.T) {
	// проверка защиты от запуска старого бинаря на новой схеме
	store, err := NewStorage(":memory:", testLogger())
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	t.Cleanup(func() {
		store.Close()
	})

	_, err = store.DB().Exec("PRAGMA user_version = 999")
	if err != nil {
		t.Fatalf("set user_version failed: %v", err)
	}

	err = Migrate(store.DB(), testLogger())
	if err == nil {
		t.Fatal("ожидалась ошибка для будущей версии схемы")
	}
	if !strings.Contains(err.Error(), "новее поддерживаемой") {
		t.Fatalf("Migrate error = %v, want future version message", err)
	}
}

func TestMigrateDropsDeprecatedCommunityBanVotingTables(t *testing.T) {
	// проверка обновления схемы версии 6 после удаления voting-таблиц
	store, err := NewStorage(":memory:", testLogger())
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	t.Cleanup(func() {
		store.Close()
	})

	for version := 1; version <= 6; version++ {
		migrationFile := fmt.Sprintf("migrations/%03d_", version)
		entries, readDirErr := migrationsFS.ReadDir("migrations")
		if readDirErr != nil {
			t.Fatalf("read migrations dir failed: %v", readDirErr)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), fmt.Sprintf("%03d_", version)) {
				migrationFile = "migrations/" + entry.Name()
				break
			}
		}

		content, readFileErr := migrationsFS.ReadFile(migrationFile)
		if readFileErr != nil {
			t.Fatalf("read migration %d failed: %v", version, readFileErr)
		}
		if _, execErr := store.DB().Exec(string(content)); execErr != nil {
			t.Fatalf("apply migration %d failed: %v", version, execErr)
		}
		if _, execErr := store.DB().Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); execErr != nil {
			t.Fatalf("set user_version %d failed: %v", version, execErr)
		}
	}

	_, err = store.DB().Exec(`
		INSERT INTO moderation_case (chat_id, spam_message_id, suspect_user_id, log_chat_id)
		VALUES (-100001, 10, 20, -100777)
	`)
	if err != nil {
		t.Fatalf("insert moderation_case failed: %v", err)
	}
	_, err = store.DB().Exec(`
		INSERT INTO moderation_vote (case_id, voter_user_id)
		VALUES (1, 30)
	`)
	if err != nil {
		t.Fatalf("insert moderation_vote failed: %v", err)
	}

	if err := Migrate(store.DB(), testLogger()); err != nil {
		t.Fatalf("Migrate from version 6 failed: %v", err)
	}

	for _, tableName := range []string{"moderation_case", "moderation_vote"} {
		if sqliteTableExists(t, store.DB(), tableName) {
			t.Fatalf("таблица %s должна быть удалена миграцией 007", tableName)
		}
	}
	for _, tableName := range []string{"chat_config", "action_log", "bot_delete_rule"} {
		if !sqliteTableExists(t, store.DB(), tableName) {
			t.Fatalf("таблица %s должна сохраниться после миграции 007", tableName)
		}
	}

	var userVersion int
	if err := store.DB().QueryRow("PRAGMA user_version").Scan(&userVersion); err != nil {
		t.Fatalf("read user_version failed: %v", err)
	}
	if userVersion != 7 {
		t.Fatalf("user_version = %d, want 7", userVersion)
	}
}

func sqliteTableExists(t *testing.T, db *sql.DB, tableName string) bool {
	t.Helper()

	var exists int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'table' AND name = ?
	`, tableName).Scan(&exists)
	if err != nil {
		t.Fatalf("check table %s failed: %v", tableName, err)
	}
	return exists > 0
}

func TestNewStorage_AppliesSQLitePragmas(t *testing.T) {
	store, err := NewStorage(":memory:", testLogger())
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	defer store.Close()

	var journalMode string
	if err := store.DB().QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode failed: %v", err)
	}
	if journalMode != "memory" {
		t.Fatalf("journal_mode = %q, want %q for :memory: DB", journalMode, "memory")
	}

	var foreignKeys int
	if err := store.DB().QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys failed: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	var busyTimeout int
	if err := store.DB().QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout failed: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}
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

			if !cfg.IsActive {
				t.Error("ожидал is_active = true после UpsertTrackedBot")
			}
		})
	}
}

func TestUpsertTrackedBot_Update(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	_, err := store.UpsertTrackedBot(chatID, "oldbot")
	if err != nil {
		t.Fatalf("первый UpsertTrackedBot failed: %v", err)
	}

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

func TestInsertActionLog(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	_, err := store.UpsertTrackedBot(chatID, "spambot")
	if err != nil {
		t.Fatalf("UpsertTrackedBot failed: %v", err)
	}

	err = store.InsertActionLog(chatID, 12345, 100, sql.NullInt64{Int64: 101, Valid: true}, ContextOrganic, ActionWarning)
	if err != nil {
		t.Fatalf("InsertActionLog failed: %v", err)
	}

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

	_, err := store.UpsertTrackedBot(chatID, "spambot")
	if err != nil {
		t.Fatalf("UpsertTrackedBot failed: %v", err)
	}

	err = store.InsertActionLog(chatID, 12345, 100, sql.NullInt64{Valid: false}, ContextOrganic, ActionWarning)
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

	if cfg.TestMode {
		t.Error("TestMode: ожидалось false по умолчанию")
	}
	if cfg.RingBufferSize != 20 {
		t.Errorf("RingBufferSize: ожидалось 20, получено %d", cfg.RingBufferSize)
	}
	if cfg.RingBufferThreshold != 2 {
		t.Errorf("RingBufferThreshold: ожидалось 2, получено %d", cfg.RingBufferThreshold)
	}
	if cfg.CommunityBanEnabled {
		t.Error("CommunityBanEnabled: ожидалось false по умолчанию")
	}
	if cfg.SpamLogChatID != 0 {
		t.Errorf("SpamLogChatID: ожидался 0, получено %d", cfg.SpamLogChatID)
	}
}

func TestSetCommunityBanEnabled(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	if err := store.SetCommunityBanEnabled(chatID, true); err != nil {
		t.Fatalf("SetCommunityBanEnabled failed: %v", err)
	}

	cfg, err := store.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}
	if !cfg.CommunityBanEnabled {
		t.Error("ожидался включённый community ban")
	}
	if !cfg.IsActive {
		t.Error("ожидался активный чат после включения community ban")
	}

	err = store.SetCommunityBanEnabled(chatID, false)
	if err != nil {
		t.Fatalf("SetCommunityBanEnabled disable failed: %v", err)
	}
	cfg, err = store.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}
	if cfg.CommunityBanEnabled {
		t.Error("ожидался выключенный community ban")
	}
}

func TestSetSpamLogChatID(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	targetChatID := int64(-100777)

	if err := store.SetSpamLogChatID(chatID, targetChatID); err != nil {
		t.Fatalf("SetSpamLogChatID failed: %v", err)
	}

	cfg, err := store.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}
	if cfg.SpamLogChatID != targetChatID {
		t.Errorf("SpamLogChatID = %d, want %d", cfg.SpamLogChatID, targetChatID)
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

	_, err := store.UpsertTrackedBot(chatID, "spambot")
	if err != nil {
		t.Fatalf("UpsertTrackedBot failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		err = store.InsertActionLog(chatID, int64(1000+i), int64(100+i),
			sql.NullInt64{Int64: int64(200 + i), Valid: true}, ContextOrganic, ActionWarning)
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

func TestActivateConfiguredChats(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	// создаём запись напрямую через SQL (эмулируя старую версию, где is_active=0)
	_, err := store.db.Exec(`
		INSERT INTO chat_config (chat_id, tracked_bot, is_active) VALUES (?, ?, 0)
	`, chatID, "spambot")
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	// проверяем что is_active=0
	cfg, _ := store.GetChatConfig(chatID)
	if cfg.IsActive {
		t.Fatal("ожидался is_active=false до миграции")
	}

	// запускаем миграцию
	err = store.ActivateConfiguredChats()
	if err != nil {
		t.Fatalf("ActivateConfiguredChats failed: %v", err)
	}

	// проверяем что is_active=1
	cfg, _ = store.GetChatConfig(chatID)
	if !cfg.IsActive {
		t.Error("ожидался is_active=true после ActivateConfiguredChats")
	}
}

func TestWithSQLiteRetry_Success(t *testing.T) {
	calls := 0
	err := withSQLiteRetry(func() error {
		calls++
		return nil
	})
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
	})
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
