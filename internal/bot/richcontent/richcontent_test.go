package richcontent

import (
	"encoding/json"
	"strings"
	"testing"

	tele "gopkg.in/telebot.v4"
)

func TestRichMessageMTSQuestionnaireSignals(t *testing.T) {
	var message tele.Message
	payload := []byte(`{
		"message_id": 10412130,
		"chat": {"id": -1001086103845, "type": "supergroup"},
		"rich_message": {
			"blocks": [{
				"type": "table",
				"is_bordered": true,
				"cells": [
					[{"text": {"type": "bold", "text": "🎉 2000₽ за прохождение короткого опроса — всем абонентам МТС."}}],
					[{"text": "📱 Пользуетесь услугами мобильной связи? Нам важно узнать ваше мнение!"}],
					[{"text": {"type": "bold", "text": "👇 Пройти опрос можно на официальном сайте"}}],
					[{"text": ["👉 ", {"type": "url", "text": "clck.su/opros", "url": "https://clck.su/opros"}]}],
					[{"text": {"type": "italic", "text": "📱 Опрос доступен только абонентам МТС"}}]
				]
			}]
		}
	}`)

	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("unmarshal rich message: %v", err)
	}

	text := Text(message.RichMessage)
	for _, want := range []string{"2000₽", "опроса", "МТС", "clck.su/opros"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Text() = %q, missing %q", text, want)
		}
	}
	if !HasLink(message.RichMessage) {
		t.Fatal("HasLink() = false, want true")
	}
}
