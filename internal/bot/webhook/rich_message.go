package webhook

import (
	"github.com/datspike/swim-bot/internal/bot/richcontent"

	tele "gopkg.in/telebot.v4"
)

// normalizeRichMessages gives telebot's existing text router a searchable view
// of rich-only messages while preserving the original RichMessage tree.
func normalizeRichMessages(update *tele.Update) {
	if update == nil {
		return
	}
	for _, message := range []*tele.Message{
		update.Message,
		update.EditedMessage,
		update.ChannelPost,
		update.EditedChannelPost,
		update.BusinessMessage,
		update.EditedBusinessMessage,
		update.GuestMessage,
	} {
		normalizeRichMessage(message)
	}
}

func normalizeRichMessage(message *tele.Message) {
	if message == nil || message.Text != "" || message.RichMessage == nil {
		return
	}
	message.Text = richcontent.Text(message.RichMessage)
}
