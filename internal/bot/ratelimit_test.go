package bot

import (
	"testing"

	"github.com/datspike/swim-bot/internal/storage"
)

func TestFormatRemaining(t *testing.T) {
	tests := []struct {
		remaining int
		want      string
	}{
		{1, "Вы можете поплавать ещё 1 раз"},
		{2, "Вы можете поплавать ещё 2 раза"},
		{3, "Вы можете поплавать ещё 3 раза"},
		{4, "Вы можете поплавать ещё 4 раза"},
		{5, "Вы можете поплавать ещё 5 раз"},
		{10, "Вы можете поплавать ещё 10 раз"},
	}

	for _, tt := range tests {
		got := formatRemaining(tt.remaining)
		if got != tt.want {
			t.Errorf("formatRemaining(%d) = %q, want %q", tt.remaining, got, tt.want)
		}
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
	}

	for _, tt := range tests {
		got := itoa(tt.input)
		if got != tt.want {
			t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// setupTestBot создаёт Bot с in-memory storage для тестов processSpam.
func setupTestBot(t *testing.T) (*Bot, *storage.Storage) {
	t.Helper()
	store := setupTestStorage(t)
	b := &Bot{
		storage: store,
		logger:  testLogger(),
	}
	return b, store
}

func TestProcessSpam_OrganicFlow(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_ = store.UpsertStickerFileID(chatID, "sticker123")

	cfg, _ := store.GetChatConfig(chatID)

	// 1-й спам: предупреждение (осталось 3)
	result, err := b.processSpam(chatID, userID, nil, cfg)
	if err != nil {
		t.Fatalf("processSpam #1 failed: %v", err)
	}
	if result.Action != storage.ActionWarning {
		t.Errorf("Action = %d, want Warning", result.Action)
	}
	if result.Remaining != 3 {
		t.Errorf("Remaining = %d, want 3", result.Remaining)
	}

	// 2-й спам: осталось 2
	result, err = b.processSpam(chatID, userID, nil, cfg)
	if err != nil {
		t.Fatalf("processSpam #2 failed: %v", err)
	}
	if result.Remaining != 2 {
		t.Errorf("Remaining = %d, want 2", result.Remaining)
	}

	// 3-й спам: осталось 1
	result, err = b.processSpam(chatID, userID, nil, cfg)
	if err != nil {
		t.Fatalf("processSpam #3 failed: %v", err)
	}
	if result.Remaining != 1 {
		t.Errorf("Remaining = %d, want 1", result.Remaining)
	}

	// 4-й спам: «наплавались» (0 осталось)
	result, err = b.processSpam(chatID, userID, nil, cfg)
	if err != nil {
		t.Fatalf("processSpam #4 failed: %v", err)
	}
	if result.Action != storage.ActionFinalWarning {
		t.Errorf("Action = %d, want FinalWarning", result.Action)
	}
	if result.Message != "Все, на сегодня наплавались" {
		t.Errorf("Message = %q, want 'Все, на сегодня наплавались'", result.Message)
	}

	// 5-й спам: кик
	result, err = b.processSpam(chatID, userID, nil, cfg)
	if err != nil {
		t.Fatalf("processSpam #5 failed: %v", err)
	}
	if result.Action != storage.ActionKick {
		t.Errorf("Action = %d, want Kick", result.Action)
	}
}

func TestProcessSpam_KickedUser(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_ = store.UpsertStickerFileID(chatID, "sticker123")

	cfg, _ := store.GetChatConfig(chatID)

	// создаём счётчик и помечаем как кикнутого
	_, _ = store.GetOrCreateSpamCounter(chatID, userID, cfg.DailyLimit)
	_ = store.MarkKicked(chatID, userID)

	// кикнутый пользователь — мгновенный кик
	result, err := b.processSpam(chatID, userID, nil, cfg)
	if err != nil {
		t.Fatalf("processSpam failed: %v", err)
	}
	if result.Action != storage.ActionKick {
		t.Errorf("Action = %d, want Kick", result.Action)
	}
}

func TestProcessSpam_CustomLimits(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_ = store.UpsertStickerFileID(chatID, "sticker123")

	// кастомный лимит: 2
	_ = store.UpdateRateLimitConfig(chatID, 2, 1, 15, 3, 5)
	cfg, _ := store.GetChatConfig(chatID)

	// 1-й спам: осталось 1
	result, _ := b.processSpam(chatID, userID, nil, cfg)
	if result.Remaining != 1 {
		t.Errorf("Remaining = %d, want 1", result.Remaining)
	}

	// 2-й спам: «наплавались»
	result, _ = b.processSpam(chatID, userID, nil, cfg)
	if result.Action != storage.ActionFinalWarning {
		t.Errorf("Action = %d, want FinalWarning", result.Action)
	}

	// 3-й спам: кик
	result, _ = b.processSpam(chatID, userID, nil, cfg)
	if result.Action != storage.ActionKick {
		t.Errorf("Action = %d, want Kick", result.Action)
	}
}
