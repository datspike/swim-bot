package storage

import (
	"path/filepath"
	"testing"
)

func TestOpenRuntimeAppliesStartupMutations(t *testing.T) {
	// startup path активирует legacy-чат с заданным tracked_bot
	dbPath := filepath.Join(t.TempDir(), "runtime.db")
	chatID := int64(-100001)

	store, err := NewStorage(dbPath, testLogger())
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	if err := Migrate(store.DB(), testLogger()); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	_, err = store.DB().Exec(`
		INSERT INTO chat_config (chat_id, tracked_bot, is_active)
		VALUES (?, 'spambot', 0)
	`, chatID)
	if err != nil {
		t.Fatalf("insert legacy chat_config failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	readyStore, err := OpenRuntime(dbPath, testLogger())
	if err != nil {
		t.Fatalf("OpenRuntime failed: %v", err)
	}
	t.Cleanup(func() {
		readyStore.Close()
	})

	cfg, err := readyStore.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("GetChatConfig = nil, want config")
	}
	if !cfg.IsActive {
		t.Fatal("IsActive = false, want true")
	}
}
