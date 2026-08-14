package bot

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/datspike/swim-bot/internal/storage"

	tele "gopkg.in/telebot.v3"
)

// isMTSQuestionnaireSpamCandidate распознаёт сообщения кампании «МТС + опрос + ссылка».
// Ссылки проверяются по entities, поэтому правило видит и URL, скрытые rich formatting.
func isMTSQuestionnaireSpamCandidate(msg *tele.Message) bool {
	if msg == nil || msg.Sender == nil || msg.Sender.IsBot || msg.Via != nil || msg.IsForwarded() {
		return false
	}

	text := strings.ToLower(strings.TrimSpace(msg.Text + "\n" + msg.Caption))
	return containsSpamToken(text, "мтс") &&
		containsSpamToken(text, "опрос") &&
		messageHasLinkEntity(msg)
}

func containsSpamToken(text, prefix string) bool {
	for _, token := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	return false
}

func messageHasLinkEntity(msg *tele.Message) bool {
	return entitiesHaveLink(msg.Entities) || entitiesHaveLink(msg.CaptionEntities)
}

func entitiesHaveLink(entities tele.Entities) bool {
	for _, entity := range entities {
		if entity.Type == tele.EntityURL {
			return true
		}
		if entity.Type == tele.EntityTextLink && strings.TrimSpace(entity.URL) != "" {
			return true
		}
	}
	return false
}

// handleMTSQuestionnaireSpam удаляет сообщение кампании и банит не-администратора.
func (b *Bot) handleMTSQuestionnaireSpam(c tele.Context, msg *tele.Message, cfg *storage.ChatConfig) error {
	chatID := c.Chat().ID
	spamMsg := &tele.Message{ID: msg.ID, Chat: c.Chat()}
	if err := c.Bot().Delete(spamMsg); err != nil {
		b.logger.Warn("mts questionnaire spam delete failed", "chat_id", chatID, "message_id", msg.ID, "error", err)
		return nil
	}

	member := &tele.ChatMember{
		User:            msg.Sender,
		RestrictedUntil: tele.Forever(),
	}
	if err := withRetry(func() error {
		return c.Bot().Ban(c.Chat(), member)
	}, b.logger); err != nil {
		b.logger.Error("mts questionnaire spam user ban failed", "chat_id", chatID, "user_id", msg.Sender.ID, "error", err)
		return nil
	}

	logReportID := 0
	if cfg.SpamLogChatID != 0 {
		logReportID = b.sendMTSQuestionnaireSpamLog(chatID, msg, cfg.SpamLogChatID)
	}
	b.logger.Info("mts questionnaire spam auto ban",
		"chat_id", chatID, "user_id", msg.Sender.ID,
		"message_id", msg.ID, "log_report_id", logReportID)
	return nil
}

func (b *Bot) sendMTSQuestionnaireSpamLog(chatID int64, msg *tele.Message, logChatID int64) int {
	text := fmt.Sprintf(
		"mts-questionnaire автобан\nЧат: %d\nСообщение: %d\nПользователь: %s (%d)\nТекст: %s",
		chatID,
		msg.ID,
		displayName(msg.Sender),
		msg.Sender.ID,
		truncateCommunityBanText(strings.TrimSpace(msg.Text+" "+msg.Caption)),
	)

	var report *tele.Message
	err := withRetry(func() error {
		var sendErr error
		report, sendErr = b.bot.Send(tele.ChatID(logChatID), text)
		return sendErr
	}, b.logger)
	if err != nil {
		b.logger.Error("mts questionnaire spam log report failed", "chat_id", chatID, "message_id", msg.ID, "target_chat_id", logChatID, "error", err)
		return 0
	}
	return report.ID
}
