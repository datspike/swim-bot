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
	_ = store.UpdateRateLimitConfig(chatID, 2, 1, 15, 3, 5, 20, 2)
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

// TestDetectContextNew_Organic проверяет органический контекст (пустой ring buffer).
func TestDetectContextNew_Organic(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	cfg, _ := store.GetChatConfig(chatID)

	ctx := b.detectContextNew(chatID, 100, cfg)
	if ctx != storage.ContextOrganic {
		t.Errorf("ожидался Organic, получено %d", ctx)
	}
}

// TestDetectContextNew_Reactive проверяет реактивный контекст (порог достигнут).
func TestDetectContextNew_Reactive(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	cfg, _ := store.GetChatConfig(chatID)

	// наполняем ring buffer спамом от других пользователей
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rb.Push(true, 200)
	rb.Push(true, 300)

	ctx := b.detectContextNew(chatID, 100, cfg)
	if ctx != storage.ContextReactive {
		t.Errorf("ожидался Reactive, получено %d", ctx)
	}
}

// TestDetectContextNew_ExcludeSelf проверяет что собственный спам не считается.
func TestDetectContextNew_ExcludeSelf(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	cfg, _ := store.GetChatConfig(chatID)

	// спам только от себя — не считается
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rb.Push(true, 100)
	rb.Push(true, 100)
	rb.Push(true, 100)

	ctx := b.detectContextNew(chatID, 100, cfg)
	if ctx != storage.ContextOrganic {
		t.Errorf("собственный спам не должен считаться: ожидался Organic, получено %d", ctx)
	}
}

// TestProcessSpamNew_OrganicFlow проверяет полный органический флоу с restrict.
func TestProcessSpamNew_OrganicFlow(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_ = store.UpsertStickerFileID(chatID, "sticker123")
	cfg, _ := store.GetChatConfig(chatID)

	// 1-й спам: предупреждение (осталось 3)
	result, err := b.processSpamNew(chatID, userID, nil, cfg)
	if err != nil {
		t.Fatalf("processSpamNew #1 failed: %v", err)
	}
	if result.Action != storage.ActionWarning {
		t.Errorf("Action = %d, want Warning", result.Action)
	}
	if result.Remaining != 3 {
		t.Errorf("Remaining = %d, want 3", result.Remaining)
	}

	// 4-й спам: restrict (вместо кика)
	for i := 2; i <= 3; i++ {
		_, _ = b.processSpamNew(chatID, userID, nil, cfg)
	}
	result, _ = b.processSpamNew(chatID, userID, nil, cfg)
	if result.Action != storage.ActionRestrict {
		t.Errorf("Action = %d, want Restrict", result.Action)
	}
	if result.Message != "Все, на сегодня наплавались" {
		t.Errorf("Message = %q, want 'Все, на сегодня наплавались'", result.Message)
	}
}

// TestProcessSpamNew_ReactiveSteal проверяет штраф в реактивном контексте.
func TestProcessSpamNew_ReactiveSteal(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_ = store.UpsertStickerFileID(chatID, "sticker123")
	cfg, _ := store.GetChatConfig(chatID)

	// наполняем ring buffer спамом от других → reactive контекст
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rb.Push(true, 200)
	rb.Push(true, 300)

	// 1-й спам в reactive: consume 1 + steal 1 = 2 потрачено, remaining = 2
	result, err := b.processSpamNew(chatID, userID, nil, cfg)
	if err != nil {
		t.Fatalf("processSpamNew failed: %v", err)
	}
	if result.Action != storage.ActionWarning {
		t.Errorf("Action = %d, want Warning", result.Action)
	}
	if result.Remaining != 2 {
		t.Errorf("Remaining = %d, want 2", result.Remaining)
	}
	if result.Message != "Группами плавать нежелательно, ворую попытку, осталось: 2" {
		t.Errorf("Message = %q", result.Message)
	}
}

// TestProcessSpamNew_ReactiveStealToZero проверяет штраф + restrict.
func TestProcessSpamNew_ReactiveStealToZero(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_ = store.UpsertStickerFileID(chatID, "sticker123")

	// лимит 2: consume 1 + steal 1 = 0 remaining
	_ = store.UpdateRateLimitConfig(chatID, 2, 1, 15, 3, 5, 20, 2)
	cfg, _ := store.GetChatConfig(chatID)

	// наполняем ring buffer
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rb.Push(true, 200)
	rb.Push(true, 300)

	result, err := b.processSpamNew(chatID, userID, nil, cfg)
	if err != nil {
		t.Fatalf("processSpamNew failed: %v", err)
	}
	if result.Action != storage.ActionRestrict {
		t.Errorf("Action = %d, want Restrict", result.Action)
	}
	if result.Message != "Группами плавать нежелательно, ворую попытку. Все, на сегодня наплавались" {
		t.Errorf("Message = %q", result.Message)
	}
}

// TestProcessSpamNew_AlreadyKicked проверяет повторный restrict для уже ограниченного.
func TestProcessSpamNew_AlreadyKicked(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	cfg, _ := store.GetChatConfig(chatID)

	_, _ = store.GetOrCreateSpamCounter(chatID, userID, cfg.DailyLimit)
	_ = store.MarkKicked(chatID, userID)

	result, _ := b.processSpamNew(chatID, userID, nil, cfg)
	if result.Action != storage.ActionRestrict {
		t.Errorf("Action = %d, want Restrict", result.Action)
	}
}
