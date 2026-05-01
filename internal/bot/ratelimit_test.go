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

func TestBuildOrganicSpamResult(t *testing.T) {
	tests := []struct {
		name       string
		count      int
		limit      int
		wantAction storage.SpamAction
		wantLeft   int
		wantText   string
	}{
		{
			name:       "warning",
			count:      1,
			limit:      4,
			wantAction: storage.ActionWarning,
			wantLeft:   3,
			wantText:   "Вы можете поплавать ещё 3 раза",
		},
		{
			name:       "restrict",
			count:      4,
			limit:      4,
			wantAction: storage.ActionRestrict,
			wantLeft:   0,
			wantText:   "Все, на сегодня наплавались",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := &storage.SpamCounter{Count: tt.count, EffectiveLimit: tt.limit}
			got := buildOrganicSpamResult(counter, 2, 3)

			if got.Action != tt.wantAction {
				t.Fatalf("Action = %d, want %d", got.Action, tt.wantAction)
			}
			if got.Remaining != tt.wantLeft {
				t.Fatalf("Remaining = %d, want %d", got.Remaining, tt.wantLeft)
			}
			if got.Message != tt.wantText {
				t.Fatalf("Message = %q, want %q", got.Message, tt.wantText)
			}
			if got.RBSpamCount != 2 || got.RBThreshold != 3 {
				t.Fatalf("ring buffer stats = %d/%d, want 2/3", got.RBSpamCount, got.RBThreshold)
			}
		})
	}
}

func TestBuildReactiveSpamResult(t *testing.T) {
	tests := []struct {
		name       string
		count      int
		limit      int
		wantAction storage.SpamAction
		wantLeft   int
		wantText   string
	}{
		{
			name:       "warning after steal",
			count:      2,
			limit:      4,
			wantAction: storage.ActionWarning,
			wantLeft:   2,
			wantText:   "Группами плавать нежелательно, ворую попытку, осталось: 2",
		},
		{
			name:       "restrict after steal",
			count:      2,
			limit:      2,
			wantAction: storage.ActionRestrict,
			wantLeft:   0,
			wantText:   "Группами плавать нежелательно, ворую попытку. Все, на сегодня наплавались",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := &storage.SpamCounter{Count: tt.count, EffectiveLimit: tt.limit}
			got := buildReactiveSpamResult(counter, 2, 3)

			if got.Action != tt.wantAction {
				t.Fatalf("Action = %d, want %d", got.Action, tt.wantAction)
			}
			if got.Remaining != tt.wantLeft {
				t.Fatalf("Remaining = %d, want %d", got.Remaining, tt.wantLeft)
			}
			if got.Message != tt.wantText {
				t.Fatalf("Message = %q, want %q", got.Message, tt.wantText)
			}
		})
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

// TestDetectContext_Organic проверяет органический контекст (пустой ring buffer).
func TestDetectContext_Organic(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	cfg, _ := store.GetChatConfig(chatID)

	ctx := b.detectContext(chatID, 100, cfg)
	if ctx != storage.ContextOrganic {
		t.Errorf("ожидался Organic, получено %d", ctx)
	}
}

// TestDetectContext_Reactive проверяет реактивный контекст (порог достигнут).
func TestDetectContext_Reactive(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	cfg, _ := store.GetChatConfig(chatID)

	// наполняем ring buffer спамом от других пользователей
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rb.Push(true, 200)
	rb.Push(true, 300)

	ctx := b.detectContext(chatID, 100, cfg)
	if ctx != storage.ContextReactive {
		t.Errorf("ожидался Reactive, получено %d", ctx)
	}
}

// TestDetectContext_ExcludeSelf проверяет что собственный спам не считается.
func TestDetectContext_ExcludeSelf(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	cfg, _ := store.GetChatConfig(chatID)

	// спам только от себя — не считается
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rb.Push(true, 100)
	rb.Push(true, 100)
	rb.Push(true, 100)

	ctx := b.detectContext(chatID, 100, cfg)
	if ctx != storage.ContextOrganic {
		t.Errorf("собственный спам не должен считаться: ожидался Organic, получено %d", ctx)
	}
}

// TestDetectContext_RBStealDisabled проверяет, что при rb_threshold <= 0 reactive-контекст отключён.
func TestDetectContext_RBStealDisabled(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	_ = store.UpdateRateLimitConfig(chatID, 4, 20, 0)
	cfg, _ := store.GetChatConfig(chatID)

	// даже при наличии спама от других пользователей контекст остаётся Organic
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rb.Push(true, 200)
	rb.Push(true, 300)

	ctx := b.detectContext(chatID, 100, cfg)
	if ctx != storage.ContextOrganic {
		t.Errorf("при rb_threshold=0 ожидался Organic, получено %d", ctx)
	}
}

// TestProcessSpam_OrganicFlow проверяет полный органический флоу с restrict.
func TestProcessSpam_OrganicFlow(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	cfg, _ := store.GetChatConfig(chatID)

	// 1-й спам: предупреждение (осталось 3)
	result, err := b.processSpam(chatID, userID, cfg)
	if err != nil {
		t.Fatalf("processSpam #1 failed: %v", err)
	}
	if result.Action != storage.ActionWarning {
		t.Errorf("Action = %d, want Warning", result.Action)
	}
	if result.Remaining != 3 {
		t.Errorf("Remaining = %d, want 3", result.Remaining)
	}

	// 4-й спам: restrict (вместо кика)
	for i := 2; i <= 3; i++ {
		_, _ = b.processSpam(chatID, userID, cfg)
	}
	result, _ = b.processSpam(chatID, userID, cfg)
	if result.Action != storage.ActionRestrict {
		t.Errorf("Action = %d, want Restrict", result.Action)
	}
	if result.Message != "Все, на сегодня наплавались" {
		t.Errorf("Message = %q, want 'Все, на сегодня наплавались'", result.Message)
	}
}

// TestProcessSpam_ReactiveSteal проверяет штраф в реактивном контексте.
func TestProcessSpam_ReactiveSteal(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	cfg, _ := store.GetChatConfig(chatID)

	// наполняем ring buffer спамом от других → reactive контекст
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rb.Push(true, 200)
	rb.Push(true, 300)

	// 1-й спам в reactive: consume 1 + steal 1 = 2 потрачено, remaining = 2
	result, err := b.processSpam(chatID, userID, cfg)
	if err != nil {
		t.Fatalf("processSpam failed: %v", err)
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

// TestProcessSpam_ReactiveStealToZero проверяет штраф + restrict.
func TestProcessSpam_ReactiveStealToZero(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")

	// лимит 2: consume 1 + steal 1 = 0 remaining
	_ = store.UpdateRateLimitConfig(chatID, 2, 20, 2)
	cfg, _ := store.GetChatConfig(chatID)

	// наполняем ring buffer
	rb := b.ringBuffers.GetOrCreate(chatID, cfg.RingBufferSize)
	rb.Push(true, 200)
	rb.Push(true, 300)

	result, err := b.processSpam(chatID, userID, cfg)
	if err != nil {
		t.Fatalf("processSpam failed: %v", err)
	}
	if result.Action != storage.ActionRestrict {
		t.Errorf("Action = %d, want Restrict", result.Action)
	}
	if result.Message != "Группами плавать нежелательно, ворую попытку. Все, на сегодня наплавались" {
		t.Errorf("Message = %q", result.Message)
	}
}

// TestProcessSpam_AlreadyKicked проверяет пропуск повторного restrict для уже ограниченного.
func TestProcessSpam_AlreadyKicked(t *testing.T) {
	b, store := setupTestBot(t)
	chatID := int64(-100001)
	userID := int64(12345)

	_, _ = store.UpsertTrackedBot(chatID, "spambot")
	cfg, _ := store.GetChatConfig(chatID)

	_, _ = store.GetOrCreateSpamCounter(chatID, userID, cfg.DailyLimit)
	_ = store.MarkKicked(chatID, userID)

	result, _ := b.processSpam(chatID, userID, cfg)
	if !result.AlreadyRestricted {
		t.Error("ожидался признак уже ограниченного пользователя")
	}
	if result.Action != 0 {
		t.Errorf("Action = %d, want 0", result.Action)
	}
}
