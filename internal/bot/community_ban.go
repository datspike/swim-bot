package bot

import (
	"fmt"
	"strings"

	"github.com/datspike/swim-bot/internal/storage"

	tele "gopkg.in/telebot.v3"
)

const (
	communityBanPromptText = "Кажется это не свободное общение"
	communityBanThanksText = "Спасибо за свободное общение!"
)

// isCommunityBanCandidate проверяет, что сообщение — ответ с цитатой из канала.
func isCommunityBanCandidate(msg *tele.Message) bool {
	if msg == nil || msg.Sender == nil || msg.Sender.IsBot {
		return false
	}
	if msg.Via != nil || msg.IsForwarded() {
		return false
	}
	if msg.ExternalReplyInfo == nil || msg.Quote == nil {
		return false
	}

	originChat := msg.ExternalReplyInfo.Chat
	if originChat == nil && msg.ExternalReplyInfo.Origin != nil {
		originChat = msg.ExternalReplyInfo.Origin.Chat
	}
	if originChat == nil {
		return false
	}

	return originChat.Type == tele.ChatChannel || originChat.Type == tele.ChatChannelPrivate
}

// handleCommunityBanDetection автоматически удаляет сообщение, банит пользователя
// и отправляет уведомление в чат и лог-чат.
func (b *Bot) handleCommunityBanDetection(c tele.Context, msg *tele.Message, cfg *storage.ChatConfig) error {
	chatID := c.Chat().ID
	senderName := displayName(msg.Sender)

	// удаление спам-сообщения
	spamMsg := &tele.Message{ID: msg.ID, Chat: c.Chat()}
	if err := c.Bot().Delete(spamMsg); err != nil {
		b.logger.Warn("community ban delete spam message failed", "chat_id", chatID, "message_id", msg.ID, "error", err)
		return nil
	}

	// бан пользователя
	banMember := &tele.ChatMember{
		User:            msg.Sender,
		RestrictedUntil: tele.Forever(),
	}
	if err := withRetry(func() error {
		return c.Bot().Ban(c.Chat(), banMember)
	}, b.logger); err != nil {
		b.logger.Error("community ban user failed", "chat_id", chatID, "user_id", msg.Sender.ID, "error", err)
		return nil
	}

	// уведомление в чат
	banText := fmt.Sprintf("%s спам цитатой каналов, автобан, %s", senderName, communityBanThanksText)
	if _, err := c.Bot().Send(c.Chat(), banText); err != nil {
		b.logger.Warn("community ban notification failed", "chat_id", chatID, "error", err)
	}

	// логирование в spam-log чат
	if cfg.SpamLogChatID != 0 {
		logReportID := b.sendCommunityBanLog(chatID, msg, cfg.SpamLogChatID, senderName)
		b.logger.Info("community ban auto",
			"chat_id", chatID, "user_id", msg.Sender.ID,
			"message_id", msg.ID, "log_report_id", logReportID)
	}

	return nil
}

// sendCommunityBanLog отправляет отчёт о бане в лог-чат.
func (b *Bot) sendCommunityBanLog(chatID int64, msg *tele.Message, logChatID int64, senderName string) int {
	logChat := tele.ChatID(logChatID)

	reportText := buildCommunityBanAutoReport(chatID, msg, senderName)
	var reportMsg *tele.Message
	err := withRetry(func() error {
		var sendErr error
		reportMsg, sendErr = b.bot.Send(logChat, reportText)
		return sendErr
	}, b.logger)
	if err != nil {
		b.logger.Error("community ban log report failed", "chat_id", chatID, "message_id", msg.ID, "target_chat_id", logChatID, "error", err)
		return 0
	}

	return reportMsg.ID
}

func buildCommunityBanAutoReport(chatID int64, msg *tele.Message, senderName string) string {
	parts := []string{
		"community-ban автобан",
		fmt.Sprintf("Чат: %d", chatID),
		fmt.Sprintf("Сообщение: %d", msg.ID),
		fmt.Sprintf("Пользователь: %s (%d)", senderName, msg.Sender.ID),
	}
	if msg.Text != "" {
		parts = append(parts, "Текст: "+truncateCommunityBanText(msg.Text))
	}
	if msg.Quote != nil && msg.Quote.Text != "" {
		parts = append(parts, "Цитата: "+truncateCommunityBanText(msg.Quote.Text))
	}
	return strings.Join(parts, "\n")
}

func truncateCommunityBanText(text string) string {
	const maxLen = 300
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.TrimSpace(text)
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}
