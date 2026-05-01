package storage

import "testing"

func TestSetBotDeleteRule_NormalizesUsername(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	if err := store.SetBotDeleteRule(chatID, "@Clown_Alert_Bot", 60); err != nil {
		t.Fatalf("SetBotDeleteRule failed: %v", err)
	}

	rule, err := store.GetBotDeleteRule(chatID, "clown_alert_bot")
	if err != nil {
		t.Fatalf("GetBotDeleteRule failed: %v", err)
	}
	if rule == nil {
		t.Fatal("ожидалось сохранённое правило")
	}
	if rule.BotUsername != "clown_alert_bot" {
		t.Fatalf("BotUsername = %q, want %q", rule.BotUsername, "clown_alert_bot")
	}
	if rule.TTLSec != 60 {
		t.Fatalf("TTLSec = %d, want %d", rule.TTLSec, 60)
	}

	cfg, err := store.GetChatConfig(chatID)
	if err != nil {
		t.Fatalf("GetChatConfig failed: %v", err)
	}
	if cfg == nil || !cfg.IsActive {
		t.Fatal("ожидался активный конфиг чата")
	}
}

func TestSetBotDeleteRule_UpdatesTTL(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	if err := store.SetBotDeleteRule(chatID, "clown_alert_bot", 60); err != nil {
		t.Fatalf("SetBotDeleteRule failed: %v", err)
	}
	if err := store.SetBotDeleteRule(chatID, "clown_alert_bot", 120); err != nil {
		t.Fatalf("SetBotDeleteRule update failed: %v", err)
	}

	rule, err := store.GetBotDeleteRule(chatID, "clown_alert_bot")
	if err != nil {
		t.Fatalf("GetBotDeleteRule failed: %v", err)
	}
	if rule == nil {
		t.Fatal("ожидалось сохранённое правило")
	}
	if rule.TTLSec != 120 {
		t.Fatalf("TTLSec = %d, want %d", rule.TTLSec, 120)
	}
}

func TestDeleteBotDeleteRule_RemovesRule(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	if err := store.SetBotDeleteRule(chatID, "clown_alert_bot", 60); err != nil {
		t.Fatalf("SetBotDeleteRule failed: %v", err)
	}
	if err := store.DeleteBotDeleteRule(chatID, "@clown_alert_bot"); err != nil {
		t.Fatalf("DeleteBotDeleteRule failed: %v", err)
	}

	rule, err := store.GetBotDeleteRule(chatID, "clown_alert_bot")
	if err != nil {
		t.Fatalf("GetBotDeleteRule failed: %v", err)
	}
	if rule != nil {
		t.Fatalf("ожидалось удалённое правило, получено %+v", rule)
	}
}

func TestListBotDeleteRules_ReturnsOnlyChatRules(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)
	otherChatID := int64(-100002)

	if err := store.SetBotDeleteRule(chatID, "beta_bot", 30); err != nil {
		t.Fatalf("SetBotDeleteRule beta failed: %v", err)
	}
	if err := store.SetBotDeleteRule(chatID, "alpha_bot", 60); err != nil {
		t.Fatalf("SetBotDeleteRule alpha failed: %v", err)
	}
	if err := store.SetBotDeleteRule(otherChatID, "other_bot", 90); err != nil {
		t.Fatalf("SetBotDeleteRule other failed: %v", err)
	}

	rules, err := store.ListBotDeleteRules(chatID)
	if err != nil {
		t.Fatalf("ListBotDeleteRules failed: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("len(rules) = %d, want %d", len(rules), 2)
	}
	want := []BotDeleteRule{
		{ChatID: chatID, BotUsername: "alpha_bot", TTLSec: 60},
		{ChatID: chatID, BotUsername: "beta_bot", TTLSec: 30},
	}
	for i, expected := range want {
		if rules[i].ChatID != expected.ChatID || rules[i].BotUsername != expected.BotUsername || rules[i].TTLSec != expected.TTLSec {
			t.Fatalf("rules[%d] = %+v, want chat=%d username=%q ttl=%d", i, rules[i], expected.ChatID, expected.BotUsername, expected.TTLSec)
		}
	}
}

func TestSetBotDeleteRule_ZeroTTLDeletesRule(t *testing.T) {
	store := setupTestDB(t)
	chatID := int64(-100001)

	if err := store.SetBotDeleteRule(chatID, "clown_alert_bot", 60); err != nil {
		t.Fatalf("SetBotDeleteRule failed: %v", err)
	}
	if err := store.SetBotDeleteRule(chatID, "clown_alert_bot", 0); err != nil {
		t.Fatalf("SetBotDeleteRule zero failed: %v", err)
	}

	rule, err := store.GetBotDeleteRule(chatID, "clown_alert_bot")
	if err != nil {
		t.Fatalf("GetBotDeleteRule failed: %v", err)
	}
	if rule != nil {
		t.Fatalf("ожидалось удалённое правило, получено %+v", rule)
	}
}

func TestNormalizeBotUsername_RejectsEmptyUsername(t *testing.T) {
	_, err := NormalizeBotUsername("@")
	if err == nil {
		t.Fatal("ожидалась ошибка для пустого username")
	}
}
