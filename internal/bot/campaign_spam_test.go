package bot

import (
	"testing"

	tele "gopkg.in/telebot.v4"
)

func TestIsMTSQuestionnaireSpamCandidate(t *testing.T) {
	spamText := "🎉 2000₽ за прохождение короткого опроса — всем абонентам МТС.\n👇 Пройти опрос можно на официальном сайте\n👉 clck.su/mtsopros"

	tests := []struct {
		name string
		msg  *tele.Message
		want bool
	}{
		{
			name: "visible URL entity",
			msg: &tele.Message{
				Sender:   &tele.User{ID: 1},
				Text:     spamText,
				Entities: tele.Entities{{Type: tele.EntityURL}},
			},
			want: true,
		},
		{
			name: "hidden rich text link",
			msg: &tele.Message{
				Sender: &tele.User{ID: 1},
				Text:   "Абонентам МТС доступен опрос на официальном сайте",
				Entities: tele.Entities{{
					Type: tele.EntityTextLink,
					URL:  "https://clck.su/mtsopros",
				}},
			},
			want: true,
		},
		{
			name: "native rich message",
			msg: &tele.Message{
				Sender: &tele.User{ID: 1},
				RichMessage: &tele.RichMessage{Blocks: []tele.RichBlock{{
					Type: tele.RichBlockTable,
					Cells: [][]tele.RichBlockTableCell{{{Text: &tele.RichText{
						Kind: tele.RichTextArray,
						Parts: []tele.RichText{
							{Kind: tele.RichTextPlain, Plain: "Абонентам МТС доступен опрос: "},
							{Kind: tele.RichTextEntity, Type: tele.RichURL, URL: "https://clck.su/opros", Text: &tele.RichText{Kind: tele.RichTextPlain, Plain: "clck.su/opros"}},
						},
					}}}},
				}}},
			},
			want: true,
		},
		{
			name: "caption link",
			msg: &tele.Message{
				Sender:  &tele.User{ID: 1},
				Caption: "МТС проводит опрос",
				CaptionEntities: tele.Entities{{
					Type: tele.EntityTextLink,
					URL:  "https://example.test/survey",
				}},
			},
			want: true,
		},
		{
			name: "missing MTS",
			msg: &tele.Message{
				Sender:   &tele.User{ID: 1},
				Text:     "Пройдите опрос",
				Entities: tele.Entities{{Type: tele.EntityURL}},
			},
		},
		{
			name: "missing questionnaire",
			msg: &tele.Message{
				Sender:   &tele.User{ID: 1},
				Text:     "Новости МТС",
				Entities: tele.Entities{{Type: tele.EntityURL}},
			},
		},
		{
			name: "missing link entity",
			msg: &tele.Message{
				Sender: &tele.User{ID: 1},
				Text:   spamText,
			},
		},
		{
			name: "empty rich link URL",
			msg: &tele.Message{
				Sender:   &tele.User{ID: 1},
				Text:     spamText,
				Entities: tele.Entities{{Type: tele.EntityTextLink}},
			},
		},
		{
			name: "bot sender",
			msg: &tele.Message{
				Sender:   &tele.User{ID: 1, IsBot: true},
				Text:     spamText,
				Entities: tele.Entities{{Type: tele.EntityURL}},
			},
		},
		{
			name: "via bot",
			msg: &tele.Message{
				Sender:   &tele.User{ID: 1},
				Via:      &tele.User{ID: 2, IsBot: true},
				Text:     spamText,
				Entities: tele.Entities{{Type: tele.EntityURL}},
			},
		},
		{
			name: "forwarded discussion",
			msg: &tele.Message{
				Sender:       &tele.User{ID: 1},
				OriginalChat: &tele.Chat{ID: -100},
				Text:         spamText,
				Entities:     tele.Entities{{Type: tele.EntityURL}},
			},
		},
		{
			name: "nil message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMTSQuestionnaireSpamCandidate(tt.msg); got != tt.want {
				t.Fatalf("isMTSQuestionnaireSpamCandidate() = %v, want %v", got, tt.want)
			}
		})
	}
}
