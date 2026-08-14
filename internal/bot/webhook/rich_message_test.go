package webhook

import (
	"encoding/json"
	"strings"
	"testing"

	tele "gopkg.in/telebot.v4"
)

func TestNormalizeRichMessagesPopulatesTextForRouting(t *testing.T) {
	var update tele.Update
	payload := []byte(`{
		"update_id": 1,
		"message": {
			"message_id": 10412130,
			"date": 1786689406,
			"from": {"id": 1, "is_bot": false, "first_name": "Spam"},
			"chat": {"id": -1001086103845, "type": "supergroup"},
			"rich_message": {
				"blocks": [{
					"type": "table",
					"cells": [
						[{"text": "🎉 2000₽ за прохождение короткого опроса — всем абонентам МТС."}],
						[{"text": ["👉 ", {"type": "url", "text": "clck.su/opros", "url": "https://clck.su/opros"}]}]
					]
				}]
			}
		}
	}`)

	if err := json.Unmarshal(payload, &update); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}
	if update.Message.Text != "" {
		t.Fatalf("Text before normalization = %q, want empty", update.Message.Text)
	}

	normalizeRichMessages(&update)

	for _, want := range []string{"2000₽", "МТС", "clck.su/opros"} {
		if !strings.Contains(update.Message.Text, want) {
			t.Fatalf("normalized Text = %q, missing %q", update.Message.Text, want)
		}
	}
}
