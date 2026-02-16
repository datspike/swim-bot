package bot

import (
	"database/sql"
	"log/slog"
	"os"
	"testing"

	"github.com/datspike/swim-bot/internal/storage"
)

// testLogger создаёт тестовый логгер.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// setupTestStorage создаёт in-memory storage для тестов.
func setupTestStorage(t *testing.T) *storage.Storage {
	t.Helper()

	store, err := storage.NewStorage(":memory:", testLogger())
	if err != nil {
		t.Fatalf("не удалось создать storage: %v", err)
	}

	if err := storage.Migrate(store.DB(), testLogger()); err != nil {
		t.Fatalf("не удалось выполнить миграции: %v", err)
	}

	t.Cleanup(func() {
		store.Close()
	})

	return store
}

func TestSpamDetection_ViaBot_StorageLogic(t *testing.T) {
	tests := []struct {
		name         string
		trackedBot   string
		viaUsername  string
		shouldDetect bool
	}{
		{
			name:         "matching via_bot",
			trackedBot:   "mlversebot",
			viaUsername:  "mlversebot",
			shouldDetect: true,
		},
		{
			name:         "wrong via_bot",
			trackedBot:   "mlversebot",
			viaUsername:  "otherbot",
			shouldDetect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupTestStorage(t)
			chatID := int64(-100001)

			// UpsertTrackedBot теперь сразу активирует
			_, err := store.UpsertTrackedBot(chatID, tt.trackedBot)
			if err != nil {
				t.Fatalf("UpsertTrackedBot failed: %v", err)
			}

			cfg, err := store.GetChatConfig(chatID)
			if err != nil {
				t.Fatalf("GetChatConfig failed: %v", err)
			}

			detected := cfg != nil &&
				cfg.IsActive &&
				cfg.TrackedBot == tt.viaUsername

			if detected != tt.shouldDetect {
				t.Errorf("detected = %v, want %v", detected, tt.shouldDetect)
			}
		})
	}
}

func TestSpamDetection_NoChatConfig(t *testing.T) {
	store := setupTestStorage(t)

	cfg, err := store.GetChatConfig(-999999)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}

	if cfg != nil {
		t.Error("ожидал nil config для ненастроенного чата")
	}
}

func TestSetBot_UsernameNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple",
			input:    "mlversebot",
			expected: "mlversebot",
		},
		{
			name:     "with @",
			input:    "@MLVerseBot",
			expected: "mlversebot",
		},
		{
			name:     "uppercase",
			input:    "SPAMBOT",
			expected: "spambot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := setupTestStorage(t)
			chatID := int64(-100001)

			_, err := store.UpsertTrackedBot(chatID, tt.input)
			if err != nil {
				t.Fatalf("UpsertTrackedBot failed: %v", err)
			}

			cfg, err := store.GetChatConfig(chatID)
			if err != nil {
				t.Fatalf("GetChatConfig failed: %v", err)
			}

			if cfg.TrackedBot != tt.expected {
				t.Errorf("TrackedBot = %q, want %q", cfg.TrackedBot, tt.expected)
			}
		})
	}
}

func TestSetBot_ActivatesBot(t *testing.T) {
	store := setupTestStorage(t)
	chatID := int64(-100001)

	_, err := store.UpsertTrackedBot(chatID, "spambot")
	if err != nil {
		t.Fatalf("UpsertTrackedBot failed: %v", err)
	}

	cfg, err := store.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}

	if !cfg.IsActive {
		t.Error("бот должен быть активен сразу после /setbot")
	}
}

func TestStats_EmptyChat(t *testing.T) {
	store := setupTestStorage(t)

	stats, err := store.GetStats(-999999)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0", stats.TotalCount)
	}

	if stats.TrackedBot != "" {
		t.Errorf("TrackedBot = %q, want empty", stats.TrackedBot)
	}

	if stats.IsActive {
		t.Error("IsActive should be false for empty chat")
	}
}

func TestStats_WithTriggers(t *testing.T) {
	store := setupTestStorage(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	// добавляем срабатывания
	for i := 0; i < 3; i++ {
		_ = store.InsertActionLog(chatID, int64(1000+i), int64(i), sql.NullInt64{Int64: int64(100 + i), Valid: true}, storage.ContextOrganic, storage.ActionWarning)
	}

	stats, err := store.GetStats(chatID)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalCount != 3 {
		t.Errorf("TotalCount = %d, want 3", stats.TotalCount)
	}

	if stats.TrackedBot != "spambot" {
		t.Errorf("TrackedBot = %q, want spambot", stats.TrackedBot)
	}

	if !stats.IsActive {
		t.Error("IsActive should be true")
	}

	if !stats.LastTrigger.Valid {
		t.Error("LastTrigger should be valid")
	}
}
