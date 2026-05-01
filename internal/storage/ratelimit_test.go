package storage

import (
	"testing"
)

func TestGetOrCreateSpamCounter_New(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	sc, err := store.GetOrCreateSpamCounter(chatID, userID, 4)
	if err != nil {
		t.Fatalf("GetOrCreateSpamCounter failed: %v", err)
	}

	if sc.Count != 0 {
		t.Errorf("Count = %d, want 0", sc.Count)
	}
	if sc.EffectiveLimit != 4 {
		t.Errorf("EffectiveLimit = %d, want 4", sc.EffectiveLimit)
	}
	if sc.Kicked {
		t.Error("Kicked should be false")
	}
}

func TestGetOrCreateSpamCounter_Existing(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	_, err := store.GetOrCreateSpamCounter(chatID, userID, 4)
	if err != nil {
		t.Fatalf("первый GetOrCreateSpamCounter failed: %v", err)
	}

	_, err = store.IncrementSpamCounter(chatID, userID)
	if err != nil {
		t.Fatalf("IncrementSpamCounter failed: %v", err)
	}

	// второй вызов не должен сбросить счётчик
	sc, err := store.GetOrCreateSpamCounter(chatID, userID, 4)
	if err != nil {
		t.Fatalf("второй GetOrCreateSpamCounter failed: %v", err)
	}

	if sc.Count != 1 {
		t.Errorf("Count = %d, want 1 (не должен сбрасываться)", sc.Count)
	}
}

func TestIncrementSpamCounter(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_, _ = store.GetOrCreateSpamCounter(chatID, userID, 4)

	for i := 0; i < 3; i++ {
		sc, err := store.IncrementSpamCounter(chatID, userID)
		if err != nil {
			t.Fatalf("IncrementSpamCounter #%d failed: %v", i+1, err)
		}
		if sc.Count != i+1 {
			t.Errorf("Count = %d, want %d", sc.Count, i+1)
		}
	}
}

func TestMarkKicked(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_, _ = store.GetOrCreateSpamCounter(chatID, userID, 4)

	err := store.MarkKicked(chatID, userID)
	if err != nil {
		t.Fatalf("MarkKicked failed: %v", err)
	}

	sc, _ := store.GetOrCreateSpamCounter(chatID, userID, 4)
	if !sc.Kicked {
		t.Error("Kicked should be true after MarkKicked")
	}
}

func TestListKickedSpamCounterUsers_ReturnsOnlyTargetDateKickedUsers(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	otherChatID := int64(-100002)
	targetDate := "2026-05-01"
	otherDate := "2026-04-30"

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_, _ = store.UpsertTrackedBot(otherChatID, "spambot")
	insertSpamCounter(t, store, chatID, 1001, targetDate, true)
	insertSpamCounter(t, store, chatID, 1002, targetDate, false)
	insertSpamCounter(t, store, chatID, 1003, otherDate, true)
	insertSpamCounter(t, store, otherChatID, 1004, targetDate, true)

	userIDs, err := store.ListKickedSpamCounterUsers(chatID, targetDate)
	if err != nil {
		t.Fatalf("ListKickedSpamCounterUsers failed: %v", err)
	}

	expectedUserIDs := []int64{1001}
	if len(userIDs) != len(expectedUserIDs) || userIDs[0] != expectedUserIDs[0] {
		t.Fatalf("userIDs = %v, want %v", userIDs, expectedUserIDs)
	}
}

func TestResetSpamCountersForDate_DeletesOnlyTargetDateAndChat(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	otherChatID := int64(-100002)
	targetDate := "2026-05-01"
	otherDate := "2026-04-30"

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_, _ = store.UpsertTrackedBot(otherChatID, "spambot")
	insertSpamCounter(t, store, chatID, 1001, targetDate, true)
	insertSpamCounter(t, store, chatID, 1002, targetDate, false)
	insertSpamCounter(t, store, chatID, 1003, otherDate, true)
	insertSpamCounter(t, store, otherChatID, 1004, targetDate, true)

	affected, err := store.ResetSpamCountersForDate(chatID, targetDate)
	if err != nil {
		t.Fatalf("ResetSpamCountersForDate failed: %v", err)
	}
	if affected != 2 {
		t.Fatalf("affected = %d, want 2", affected)
	}

	remaining := countSpamCounters(t, store)
	if remaining != 2 {
		t.Fatalf("remaining counters = %d, want 2", remaining)
	}
}

func insertSpamCounter(t *testing.T, store *Storage, chatID, userID int64, date string, kicked bool) {
	t.Helper()

	kickedValue := 0
	if kicked {
		kickedValue = 1
	}
	_, err := store.DB().Exec(`
		INSERT INTO spam_counter (chat_id, user_id, date, count, effective_limit, kicked)
		VALUES (?, ?, ?, 1, 4, ?)
	`, chatID, userID, date, kickedValue)
	if err != nil {
		t.Fatalf("insert spam_counter failed: %v", err)
	}
}

func countSpamCounters(t *testing.T, store *Storage) int {
	t.Helper()

	var count int
	if err := store.DB().QueryRow("SELECT COUNT(*) FROM spam_counter").Scan(&count); err != nil {
		t.Fatalf("count spam_counter failed: %v", err)
	}
	return count
}

func TestUpdateRateLimitConfig(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	err := store.UpdateRateLimitConfig(chatID, 6, 30, 3)
	if err != nil {
		t.Fatalf("UpdateRateLimitConfig failed: %v", err)
	}

	cfg, _ := store.GetChatConfig(chatID)
	if cfg.DailyLimit != 6 {
		t.Errorf("DailyLimit = %d, want 6", cfg.DailyLimit)
	}
	if cfg.RingBufferSize != 30 {
		t.Errorf("RingBufferSize = %d, want 30", cfg.RingBufferSize)
	}
	if cfg.RingBufferThreshold != 3 {
		t.Errorf("RingBufferThreshold = %d, want 3", cfg.RingBufferThreshold)
	}
}

func TestSetTestMode(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	// включаем тестовый режим
	err := store.SetTestMode(chatID, true)
	if err != nil {
		t.Fatalf("SetTestMode(true) failed: %v", err)
	}
	cfg, _ := store.GetChatConfig(chatID)
	if !cfg.TestMode {
		t.Error("TestMode: ожидалось true")
	}

	// выключаем
	err = store.SetTestMode(chatID, false)
	if err != nil {
		t.Fatalf("SetTestMode(false) failed: %v", err)
	}
	cfg, _ = store.GetChatConfig(chatID)
	if cfg.TestMode {
		t.Error("TestMode: ожидалось false")
	}
}

func TestChatConfig_DefaultRateLimits(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	cfg, err := store.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}

	if cfg.DailyLimit != 4 {
		t.Errorf("DailyLimit = %d, want 4 (default)", cfg.DailyLimit)
	}
	if cfg.RingBufferSize != 20 {
		t.Errorf("RingBufferSize = %d, want 20 (default)", cfg.RingBufferSize)
	}
	if cfg.RingBufferThreshold != 2 {
		t.Errorf("RingBufferThreshold = %d, want 2 (default)", cfg.RingBufferThreshold)
	}
}
