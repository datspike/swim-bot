package storage

import (
	"database/sql"
	"testing"
)

func TestGetOrCreateSpamCounter_New(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	userID := int64(12345)

	// создаём конфиг (для foreign key)
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

	// создаём первый раз
	_, err := store.GetOrCreateSpamCounter(chatID, userID, 4)
	if err != nil {
		t.Fatalf("первый GetOrCreateSpamCounter failed: %v", err)
	}

	// инкрементируем
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

	// три инкремента
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

func TestUpdateEffectiveLimit(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_, _ = store.GetOrCreateSpamCounter(chatID, userID, 4)

	// снижаем лимит
	err := store.UpdateEffectiveLimit(chatID, userID, 1)
	if err != nil {
		t.Fatalf("UpdateEffectiveLimit failed: %v", err)
	}

	sc, _ := store.GetOrCreateSpamCounter(chatID, userID, 4)
	if sc.EffectiveLimit != 1 {
		t.Errorf("EffectiveLimit = %d, want 1", sc.EffectiveLimit)
	}
}

func TestUpdateEffectiveLimit_MinimumOne(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_, _ = store.GetOrCreateSpamCounter(chatID, userID, 4)

	// попытка установить 0 — должен стать 1
	err := store.UpdateEffectiveLimit(chatID, userID, 0)
	if err != nil {
		t.Fatalf("UpdateEffectiveLimit failed: %v", err)
	}

	sc, _ := store.GetOrCreateSpamCounter(chatID, userID, 4)
	if sc.EffectiveLimit != 1 {
		t.Errorf("EffectiveLimit = %d, want 1 (минимум)", sc.EffectiveLimit)
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

func TestSpamCountSince(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	// добавляем 5 записей в action_log
	for i := 0; i < 5; i++ {
		_ = store.InsertActionLog(chatID, int64(1000+i), int64(100+i),
			sql.NullInt64{Int64: int64(200 + i), Valid: true}, ContextOrganic, ActionWarning)
	}

	count, err := store.SpamCountSince(chatID, 5)
	if err != nil {
		t.Fatalf("SpamCountSince failed: %v", err)
	}

	if count != 5 {
		t.Errorf("Count = %d, want 5", count)
	}
}

func TestSpamCountSince_Empty(t *testing.T) {
	store := setupTestDB(t)

	count, err := store.SpamCountSince(-999999, 5)
	if err != nil {
		t.Fatalf("SpamCountSince failed: %v", err)
	}

	if count != 0 {
		t.Errorf("Count = %d, want 0", count)
	}
}

func TestHasRecentSpamByOther(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	currentUserID := int64(12345)
	otherUserID := int64(67890)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	// спам от другого пользователя
	_ = store.InsertActionLog(chatID, otherUserID, 100,
		sql.NullInt64{Valid: false}, ContextOrganic, ActionWarning)

	has, err := store.HasRecentSpamByOther(chatID, currentUserID, 15)
	if err != nil {
		t.Fatalf("HasRecentSpamByOther failed: %v", err)
	}

	if !has {
		t.Error("должен обнаружить спам от другого пользователя")
	}
}

func TestHasRecentSpamByOther_SameUser(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	// спам от того же пользователя — не должен считаться
	_ = store.InsertActionLog(chatID, userID, 100,
		sql.NullInt64{Valid: false}, ContextOrganic, ActionWarning)

	has, err := store.HasRecentSpamByOther(chatID, userID, 15)
	if err != nil {
		t.Fatalf("HasRecentSpamByOther failed: %v", err)
	}

	if has {
		t.Error("не должен считать спам от того же пользователя")
	}
}

func TestUpsertTrackedStickerPack(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	err := store.UpsertTrackedStickerPack(chatID, "AnimatedEmojis")
	if err != nil {
		t.Fatalf("UpsertTrackedStickerPack failed: %v", err)
	}

	cfg, _ := store.GetChatConfig(chatID)
	if cfg.TrackedStickerPack != "animatedemojis" {
		t.Errorf("TrackedStickerPack = %q, want %q", cfg.TrackedStickerPack, "animatedemojis")
	}
}

func TestUpsertTrackedStickerPack_Replace(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	_ = store.UpsertTrackedStickerPack(chatID, "OldPack")
	_ = store.UpsertTrackedStickerPack(chatID, "NewPack")

	cfg, _ := store.GetChatConfig(chatID)
	if cfg.TrackedStickerPack != "newpack" {
		t.Errorf("TrackedStickerPack = %q, want %q", cfg.TrackedStickerPack, "newpack")
	}
}

func TestUpdateRateLimitConfig(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	err := store.UpdateRateLimitConfig(chatID, 6, 2, 10, 5, 3)
	if err != nil {
		t.Fatalf("UpdateRateLimitConfig failed: %v", err)
	}

	cfg, _ := store.GetChatConfig(chatID)
	if cfg.DailyLimit != 6 {
		t.Errorf("DailyLimit = %d, want 6", cfg.DailyLimit)
	}
	if cfg.ReactiveLimit != 2 {
		t.Errorf("ReactiveLimit = %d, want 2", cfg.ReactiveLimit)
	}
	if cfg.ReactiveWindowMin != 10 {
		t.Errorf("ReactiveWindowMin = %d, want 10", cfg.ReactiveWindowMin)
	}
	if cfg.SpamDensityThreshold != 5 {
		t.Errorf("SpamDensityThreshold = %d, want 5", cfg.SpamDensityThreshold)
	}
	if cfg.SpamDensityWindowMin != 3 {
		t.Errorf("SpamDensityWindowMin = %d, want 3", cfg.SpamDensityWindowMin)
	}
}

func TestChatConfig_DefaultRateLimits(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	// создаём конфиг только с tracked_bot
	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	cfg, err := store.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}

	// проверяем дефолтные значения из миграции
	if cfg.DailyLimit != 4 {
		t.Errorf("DailyLimit = %d, want 4 (default)", cfg.DailyLimit)
	}
	if cfg.ReactiveLimit != 1 {
		t.Errorf("ReactiveLimit = %d, want 1 (default)", cfg.ReactiveLimit)
	}
	if cfg.ReactiveWindowMin != 15 {
		t.Errorf("ReactiveWindowMin = %d, want 15 (default)", cfg.ReactiveWindowMin)
	}
	if cfg.SpamDensityThreshold != 3 {
		t.Errorf("SpamDensityThreshold = %d, want 3 (default)", cfg.SpamDensityThreshold)
	}
	if cfg.SpamDensityWindowMin != 5 {
		t.Errorf("SpamDensityWindowMin = %d, want 5 (default)", cfg.SpamDensityWindowMin)
	}
	if cfg.TrackedStickerPack != "" {
		t.Errorf("TrackedStickerPack = %q, want empty (default)", cfg.TrackedStickerPack)
	}
}
