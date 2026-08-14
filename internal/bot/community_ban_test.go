package bot

import (
	"testing"

	"github.com/datspike/swim-bot/internal/storage"

	tele "gopkg.in/telebot.v4"
)

func TestIsCommunityBanCandidate(t *testing.T) {
	msg := &tele.Message{
		Sender: &tele.User{ID: 1},
		Chat:   &tele.Chat{ID: -100, Type: tele.ChatSuperGroup},
		ExternalReply: &tele.ExternalReply{
			Chat: &tele.Chat{ID: -200, Type: tele.ChatChannel},
		},
		Quote: &tele.TextQuote{Text: "vpn ad"},
	}

	if !isCommunityBanCandidate(msg) {
		t.Fatal("ожидалась community-ban сигнатура")
	}
}

func TestIsCommunityBanCandidateRejectsForwardAndVia(t *testing.T) {
	msg := &tele.Message{
		Sender: &tele.User{ID: 1},
		Chat:   &tele.Chat{ID: -100, Type: tele.ChatSuperGroup},
		ExternalReply: &tele.ExternalReply{
			Chat: &tele.Chat{ID: -200, Type: tele.ChatChannel},
		},
		Quote:        &tele.TextQuote{Text: "vpn ad"},
		Via:          &tele.User{ID: 2, Username: "helperbot"},
		OriginalChat: &tele.Chat{ID: -300, Type: tele.ChatChannel},
	}

	if isCommunityBanCandidate(msg) {
		t.Fatal("не ожидалась community-ban сигнатура для via/forward")
	}
}

func TestIsCommunityBanCandidateRejectsNonChannelOrigin(t *testing.T) {
	msg := &tele.Message{
		Sender: &tele.User{ID: 1},
		Chat:   &tele.Chat{ID: -100, Type: tele.ChatSuperGroup},
		ExternalReply: &tele.ExternalReply{
			Chat: &tele.Chat{ID: -200, Type: tele.ChatSuperGroup},
		},
		Quote: &tele.TextQuote{Text: "quote"},
	}

	if isCommunityBanCandidate(msg) {
		t.Fatal("не ожидалась community-ban сигнатура для не-channel origin")
	}
}

func TestShouldCommunityBan(t *testing.T) {
	// решение учитывает настройку, сигнатуру сообщения и роль отправителя
	candidate := &tele.Message{
		Sender: &tele.User{ID: 1},
		Chat:   &tele.Chat{ID: -100, Type: tele.ChatSuperGroup},
		ExternalReply: &tele.ExternalReply{
			Chat: &tele.Chat{ID: -200, Type: tele.ChatChannel},
		},
		Quote: &tele.TextQuote{Text: "vpn ad"},
	}
	cfg := &storage.ChatConfig{CommunityBanEnabled: true}

	tests := []struct {
		name string
		msg  *tele.Message
		cfg  *storage.ChatConfig
		role tele.MemberStatus
		want bool
	}{
		{name: "member candidate", msg: candidate, cfg: cfg, role: tele.Member, want: true},
		{name: "disabled", msg: candidate, cfg: &storage.ChatConfig{}, role: tele.Member},
		{name: "nil config", msg: candidate, role: tele.Member},
		{name: "nil message", cfg: cfg, role: tele.Member},
		{name: "administrator exempt", msg: candidate, cfg: cfg, role: tele.Administrator},
		{name: "creator exempt", msg: candidate, cfg: cfg, role: tele.Creator},
		{name: "non candidate", msg: &tele.Message{Sender: &tele.User{ID: 1}}, cfg: cfg, role: tele.Member},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldCommunityBan(tt.msg, tt.cfg, tt.role)
			if got != tt.want {
				t.Fatalf("shouldCommunityBan() = %v, want %v", got, tt.want)
			}
		})
	}
}
