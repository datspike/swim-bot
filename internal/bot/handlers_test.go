package bot

import (
	"database/sql"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/datspike/swim-bot/internal/storage"

	tele "gopkg.in/telebot.v3"
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

func TestParseChatIDArg(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantChatID  int64
		wantOK      bool
		wantMessage string
	}{
		{
			name:        "missing chat id",
			args:        nil,
			wantMessage: "usage",
		},
		{
			name:        "invalid chat id",
			args:        []string{"abc"},
			wantMessage: "invalid",
		},
		{
			name:       "valid chat id",
			args:       []string{"-100123"},
			wantChatID: -100123,
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatID, ok, message := parseChatIDArg(tt.args, "usage", "invalid")
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if chatID != tt.wantChatID {
				t.Fatalf("chatID = %d, want %d", chatID, tt.wantChatID)
			}
			if message != tt.wantMessage {
				t.Fatalf("message = %q, want %q", message, tt.wantMessage)
			}
		})
	}
}

func TestValidateRateLimitConfig(t *testing.T) {
	tests := []struct {
		name        string
		daily       int
		rbSize      int
		rbThreshold int
		wantErr     bool
	}{
		{name: "valid", daily: 1, rbSize: 1, rbThreshold: 0},
		{name: "negative threshold", daily: 4, rbSize: 20, rbThreshold: -1, wantErr: true},
		{name: "zero daily", daily: 0, rbSize: 20, rbThreshold: 2, wantErr: true},
		{name: "zero ring buffer", daily: 4, rbSize: 0, rbThreshold: 2, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRateLimitConfig(tt.daily, tt.rbSize, tt.rbThreshold)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateRateLimitConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestShouldDeleteAdminSpam(t *testing.T) {
	tests := []struct {
		name   string
		role   tele.MemberStatus
		ttlSec int
		want   bool
	}{
		{
			name:   "administrator with ttl",
			role:   tele.Administrator,
			ttlSec: 60,
			want:   true,
		},
		{
			name:   "creator with ttl",
			role:   tele.Creator,
			ttlSec: 60,
			want:   true,
		},
		{
			name:   "administrator without ttl",
			role:   tele.Administrator,
			ttlSec: 0,
			want:   false,
		},
		{
			name:   "member with ttl",
			role:   tele.Member,
			ttlSec: 60,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldDeleteAdminSpam(tt.role, tt.ttlSec)
			if got != tt.want {
				t.Fatalf("shouldDeleteAdminSpam(%q, %d) = %v, want %v", tt.role, tt.ttlSec, got, tt.want)
			}
		})
	}
}

func TestIsMessageOlderThan(t *testing.T) {
	now := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		messageTime time.Time
		want        bool
	}{
		{
			name:        "inside age",
			messageTime: now.Add(-29 * time.Second),
		},
		{
			name:        "equal age",
			messageTime: now.Add(-30 * time.Second),
		},
		{
			name:        "older than age",
			messageTime: now.Add(-31 * time.Second),
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMessageOlderThan(tt.messageTime, 30*time.Second, now)
			if got != tt.want {
				t.Fatalf("isMessageOlderThan() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTrackedBotSpam(t *testing.T) {
	tests := []struct {
		name       string
		message    *tele.Message
		trackedBot string
		want       bool
	}{
		{
			name:       "matching via bot",
			message:    &tele.Message{Sender: &tele.User{ID: 1}, Via: &tele.User{Username: "MLVerseBot"}},
			trackedBot: "mlversebot",
			want:       true,
		},
		{
			name:       "wrong via bot",
			message:    &tele.Message{Sender: &tele.User{ID: 1}, Via: &tele.User{Username: "otherbot"}},
			trackedBot: "mlversebot",
		},
		{
			name:       "nil sender",
			message:    &tele.Message{Via: &tele.User{Username: "mlversebot"}},
			trackedBot: "mlversebot",
		},
		{
			name:       "nil via",
			message:    &tele.Message{Sender: &tele.User{ID: 1}},
			trackedBot: "mlversebot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTrackedBotSpam(tt.message, tt.trackedBot)
			if got != tt.want {
				t.Fatalf("isTrackedBotSpam() = %v, want %v", got, tt.want)
			}
		})
	}
}

type recordingScheduler struct {
	now    time.Time
	delays []time.Duration
}

func (s *recordingScheduler) Now() time.Time {
	return s.now
}

func (s *recordingScheduler) AfterFunc(delay time.Duration, _ func()) {
	s.delays = append(s.delays, delay)
}

func TestBuildUnrestrictChatMemberAt(t *testing.T) {
	now := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	member := buildUnrestrictChatMemberAt(123, now)
	wantUntil := now.Add(unrestrictStatusClearDelay).Unix()

	if member.User.ID != 123 {
		t.Fatalf("User.ID = %d, want 123", member.User.ID)
	}
	if member.RestrictedUntil != wantUntil {
		t.Fatalf("RestrictedUntil = %d, want %d", member.RestrictedUntil, wantUntil)
	}
	if !member.Independent {
		t.Fatalf("Rights.Independent = false, want true")
	}
}

func TestEndOfDayUTCAt(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "normal day",
			now:  time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
			want: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "less than thirty seconds",
			now:  time.Date(2026, 5, 1, 23, 59, 45, 0, time.UTC),
			want: time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := endOfDayUTCAt(tt.now)
			if got != tt.want.Unix() {
				t.Fatalf("endOfDayUTCAt() = %d, want %d", got, tt.want.Unix())
			}
		})
	}
}

func TestScheduleMessageDeleteUsesScheduler(t *testing.T) {
	scheduler := &recordingScheduler{now: time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)}
	bot := &Bot{scheduler: scheduler, logger: testLogger()}

	bot.scheduleMessageDelete(-100, 42, 15*time.Second, "test")

	if len(scheduler.delays) != 1 {
		t.Fatalf("scheduled deletes = %d, want 1", len(scheduler.delays))
	}
	if scheduler.delays[0] != 15*time.Second {
		t.Fatalf("delay = %s, want 15s", scheduler.delays[0])
	}
}

func TestShouldAutoDeleteSpamReply(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "remaining message",
			text: "Вы можете поплавать ещё 3 раза",
			want: true,
		},
		{
			name: "test mode remaining message",
			text: "[ТЕСТ 1/4 rb:0/2] Вы можете поплавать ещё 3 раза",
			want: true,
		},
		{
			name: "restrict message",
			text: "Все, на сегодня наплавались",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldAutoDeleteSpamReply(tt.text)
			if got != tt.want {
				t.Fatalf("shouldAutoDeleteSpamReply(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestShouldDeleteBotMessage_ConfiguredBotWithTTL(t *testing.T) {
	msg := &tele.Message{Sender: &tele.User{IsBot: true, Username: "Clown_Alert_Bot"}}
	rule := &storage.BotDeleteRule{BotUsername: "clown_alert_bot", TTLSec: 60}

	if !shouldDeleteBotMessage(msg, rule) {
		t.Fatal("ожидалось автоудаление для настроенного bot-аккаунта")
	}
}

func TestShouldDeleteBotMessage_RegularUserIgnored(t *testing.T) {
	msg := &tele.Message{Sender: &tele.User{IsBot: false, Username: "clown_alert_bot"}}
	rule := &storage.BotDeleteRule{BotUsername: "clown_alert_bot", TTLSec: 60}

	if shouldDeleteBotMessage(msg, rule) {
		t.Fatal("не ожидалось автоудаление для обычного пользователя")
	}
}

func TestShouldDeleteBotMessage_EmptyUsernameIgnored(t *testing.T) {
	msg := &tele.Message{Sender: &tele.User{IsBot: true}}
	rule := &storage.BotDeleteRule{BotUsername: "clown_alert_bot", TTLSec: 60}

	if shouldDeleteBotMessage(msg, rule) {
		t.Fatal("не ожидалось автоудаление для bot-аккаунта без username")
	}
}

func TestShouldDeleteBotMessage_NoRuleIgnored(t *testing.T) {
	msg := &tele.Message{Sender: &tele.User{IsBot: true, Username: "clown_alert_bot"}}

	if shouldDeleteBotMessage(msg, nil) {
		t.Fatal("не ожидалось автоудаление без правила")
	}
}

func TestShouldDeleteBotMessage_ZeroTTLIgnored(t *testing.T) {
	msg := &tele.Message{Sender: &tele.User{IsBot: true, Username: "clown_alert_bot"}}
	rule := &storage.BotDeleteRule{BotUsername: "clown_alert_bot", TTLSec: 0}

	if shouldDeleteBotMessage(msg, rule) {
		t.Fatal("не ожидалось автоудаление при ttl=0")
	}
}

func TestUniqueInt64s_RemovesDuplicatesKeepsOrder(t *testing.T) {
	values := []int64{1001, 1002, 1001, 1003, 1002}

	got := uniqueInt64s(values)
	want := []int64{1001, 1002, 1003}
	if len(got) != len(want) {
		t.Fatalf("uniqueInt64s() = %v, want %v", got, want)
	}
	for index, expectedValue := range want {
		if got[index] != expectedValue {
			t.Fatalf("uniqueInt64s() = %v, want %v", got, want)
		}
	}
}

func TestBuildUnrestrictChatMember_AllowsOtherMessagesIndependently(t *testing.T) {
	userID := int64(1001)
	before := time.Now().Add(unrestrictStatusClearDelay - time.Second).Unix()

	member := buildUnrestrictChatMember(userID)
	after := time.Now().Add(unrestrictStatusClearDelay + time.Second).Unix()

	if member.User == nil || member.User.ID != userID {
		t.Fatalf("User.ID = %v, want %d", member.User, userID)
	}
	if !member.CanSendOther {
		t.Fatal("CanSendOther = false, want true")
	}
	if !member.Independent {
		t.Fatal("Independent = false, want true")
	}
	if member.RestrictedUntil < before || member.RestrictedUntil > after {
		t.Fatalf("RestrictedUntil = %d, want between %d and %d", member.RestrictedUntil, before, after)
	}
}

func TestFormatResetCountersResponse_IncludesResetAndRestrictionCounts(t *testing.T) {
	response := formatResetCountersResponse(3, -100001, resetRestrictionsResult{
		Candidates: 2,
		Succeeded:  1,
		Failed:     1,
	})

	for _, expectedText := range []string{"Сброшено 3", "чата -100001", "найдено 2", "снято 1", "ошибок 1"} {
		if !strings.Contains(response, expectedText) {
			t.Fatalf("response = %q, want substring %q", response, expectedText)
		}
	}
}
