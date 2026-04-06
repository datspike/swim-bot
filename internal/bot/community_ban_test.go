package bot

import (
	"testing"

	tele "gopkg.in/telebot.v3"
)

func TestIsCommunityBanCandidate(t *testing.T) {
	msg := &tele.Message{
		Sender: &tele.User{ID: 1},
		Chat:   &tele.Chat{ID: -100, Type: tele.ChatSuperGroup},
		ExternalReplyInfo: &tele.ExternalReplyInfo{
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
		ExternalReplyInfo: &tele.ExternalReplyInfo{
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
		ExternalReplyInfo: &tele.ExternalReplyInfo{
			Chat: &tele.Chat{ID: -200, Type: tele.ChatSuperGroup},
		},
		Quote: &tele.TextQuote{Text: "quote"},
	}

	if isCommunityBanCandidate(msg) {
		t.Fatal("не ожидалась community-ban сигнатура для не-channel origin")
	}
}
