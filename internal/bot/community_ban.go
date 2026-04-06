package bot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/datspike/swim-bot/internal/storage"

	tele "gopkg.in/telebot.v3"
)

const (
	communityBanVoteUnique = "community_ban_vote"
	communityBanPromptText = "Кажется это не свободное общение"
	communityBanThanksText = "Спасибо за свободное общение!"
	communityBanClosedText = "Похоже, сообщение уже удалили вручную"
	communityBanQuorum     = 3
)

func (b *Bot) buildCommunityBanMarkup(caseID int64, votes int) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	button := markup.Data(fmt.Sprintf("Ban %d/%d", votes, communityBanQuorum), communityBanVoteUnique, strconv.FormatInt(caseID, 10))
	markup.Inline(markup.Row(button))
	return markup
}

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

func (b *Bot) handleCommunityBanDetection(c tele.Context, msg *tele.Message, cfg *storage.ChatConfig) error {
	caseItem, err := b.storage.CreateModerationCase(c.Chat().ID, int64(msg.ID), msg.Sender.ID, cfg.SpamLogChatID)
	if err != nil {
		b.logger.Error("create moderation case failed", "chat_id", c.Chat().ID, "message_id", msg.ID, "error", err)
		return nil
	}
	if caseItem == nil {
		return nil
	}
	if caseItem.BotReplyMessageID.Valid {
		return nil
	}

	replyMarkup := b.buildCommunityBanMarkup(caseItem.ID, 0)
	var replyMsg *tele.Message
	err = withRetry(func() error {
		var sendErr error
		replyMsg, sendErr = c.Bot().Reply(msg, communityBanPromptText, replyMarkup)
		return sendErr
	}, b.logger)
	if err != nil {
		b.logger.Error("community ban reply failed", "chat_id", c.Chat().ID, "message_id", msg.ID, "error", err)
		return nil
	}

	logReportID := 0
	if cfg.SpamLogChatID != 0 {
		logReportID = b.sendCommunityBanLog(c.Chat(), msg, cfg.SpamLogChatID)
	}

	if err := b.storage.SetModerationCaseMessages(caseItem.ID, replyMsg.ID, logReportID); err != nil {
		b.logger.Error("save moderation case messages failed", "case_id", caseItem.ID, "error", err)
	}

	return nil
}

func (b *Bot) handleCommunityBanVote(c tele.Context) error {
	caseID, err := strconv.ParseInt(c.Data(), 10, 64)
	if err != nil {
		return c.RespondAlert("Некорректный идентификатор голосования")
	}

	caseItem, err := b.storage.GetModerationCase(caseID)
	if err != nil {
		b.logger.Error("get moderation case failed", "case_id", caseID, "error", err)
		return c.RespondAlert("Не удалось обработать голос")
	}
	if caseItem == nil {
		return c.RespondAlert("Голосование не найдено")
	}
	if caseItem.Status != storage.CommunityBanStatusOpen {
		return c.RespondText("Голосование уже закрыто")
	}

	member, err := c.Bot().ChatMemberOf(c.Chat(), c.Sender())
	if err != nil {
		b.logger.Warn("community ban voter membership check failed", "chat_id", c.Chat().ID, "user_id", c.Sender().ID, "error", err)
		return c.RespondAlert("Не удалось проверить участие в чате")
	}
	if member.Role == tele.Left || member.Role == tele.Kicked {
		return c.RespondAlert("Голосовать могут только участники чата")
	}

	votes, added, err := b.storage.AddModerationVote(caseID, c.Sender().ID)
	if err != nil {
		b.logger.Error("save moderation vote failed", "case_id", caseID, "user_id", c.Sender().ID, "error", err)
		return c.RespondAlert("Не удалось сохранить голос")
	}
	if !added {
		return c.RespondText("Ваш голос уже учтён")
	}

	if votes < communityBanQuorum {
		if err := c.Edit(communityBanPromptText, b.buildCommunityBanMarkup(caseID, votes)); err != nil {
			b.logger.Warn("community ban vote edit failed", "case_id", caseID, "votes", votes, "error", err)
		}
		return c.RespondText(fmt.Sprintf("Голос учтён: %d/%d", votes, communityBanQuorum))
	}

	spamMsg := &tele.Message{ID: int(caseItem.SpamMessageID), Chat: c.Chat()}
	if err := c.Bot().Delete(spamMsg); err != nil {
		b.logger.Warn("community ban delete spam message failed", "case_id", caseID, "message_id", caseItem.SpamMessageID, "error", err)
		if closeErr := b.storage.CloseModerationCase(caseID); closeErr != nil {
			b.logger.Error("close moderation case failed", "case_id", caseID, "error", closeErr)
		}
		if editErr := c.Edit(communityBanClosedText); editErr != nil {
			b.logger.Warn("community ban close edit failed", "case_id", caseID, "error", editErr)
		}
		b.updateCommunityBanLog(caseItem, communityBanClosedText)
		return c.RespondText("Сообщение уже удалено")
	}

	banMember := &tele.ChatMember{
		User:            &tele.User{ID: caseItem.SuspectUserID},
		RestrictedUntil: tele.Forever(),
	}
	if err := withRetry(func() error {
		return c.Bot().Ban(c.Chat(), banMember)
	}, b.logger); err != nil {
		b.logger.Error("community ban user failed", "case_id", caseID, "user_id", caseItem.SuspectUserID, "error", err)
		return c.RespondAlert("Не удалось забанить пользователя")
	}

	if err := b.storage.MarkModerationCaseBanned(caseID); err != nil {
		b.logger.Error("mark moderation case banned failed", "case_id", caseID, "error", err)
	}
	if err := c.Edit(communityBanThanksText); err != nil {
		b.logger.Warn("community ban final edit failed", "case_id", caseID, "error", err)
	}
	b.updateCommunityBanLog(caseItem, communityBanThanksText)

	return c.RespondText("Порог голосов достигнут")
}

func (b *Bot) sendCommunityBanLog(sourceChat *tele.Chat, msg *tele.Message, logChatID int64) int {
	logChat := tele.ChatID(logChatID)
	forwardState := "forward_ok"

	if err := withRetry(func() error {
		_, sendErr := b.bot.Forward(logChat, msg)
		return sendErr
	}, b.logger); err != nil {
		forwardState = "forward_failed"
		if copyErr := withRetry(func() error {
			_, sendErr := b.bot.Copy(logChat, msg)
			return sendErr
		}, b.logger); copyErr != nil {
			forwardState = "copy_failed"
			b.logger.Warn("community ban log forward/copy failed", "chat_id", sourceChat.ID, "message_id", msg.ID, "target_chat_id", logChatID, "forward_error", err, "copy_error", copyErr)
		}
	}

	reportText := buildCommunityBanReport(sourceChat, msg, forwardState, communityBanPromptText)
	var reportMsg *tele.Message
	err := withRetry(func() error {
		var sendErr error
		reportMsg, sendErr = b.bot.Send(logChat, reportText)
		return sendErr
	}, b.logger)
	if err != nil {
		b.logger.Error("community ban log report failed", "chat_id", sourceChat.ID, "message_id", msg.ID, "target_chat_id", logChatID, "error", err)
		return 0
	}

	return reportMsg.ID
}

func (b *Bot) updateCommunityBanLog(caseItem *storage.ModerationCase, finalStatus string) {
	if caseItem == nil || caseItem.LogChatID == 0 || !caseItem.LogReportMessageID.Valid {
		return
	}

	reportMsg := &tele.Message{ID: int(caseItem.LogReportMessageID.Int64), Chat: &tele.Chat{ID: caseItem.LogChatID}}
	text := buildCommunityBanFinalReport(caseItem, finalStatus)
	if err := withRetry(func() error {
		_, editErr := b.bot.Edit(reportMsg, text)
		return editErr
	}, b.logger); err != nil {
		b.logger.Warn("community ban log update failed", "case_id", caseItem.ID, "error", err)
	}
}

func buildCommunityBanReport(sourceChat *tele.Chat, msg *tele.Message, forwardState string, status string) string {
	parts := []string{
		"community-ban детект",
		fmt.Sprintf("Чат: %s (%d)", communityBanChatName(sourceChat), sourceChat.ID),
		fmt.Sprintf("Сообщение: %d", msg.ID),
		fmt.Sprintf("Пользователь: %s (%d)", displayName(msg.Sender), msg.Sender.ID),
		fmt.Sprintf("Доставка копии: %s", forwardState),
		fmt.Sprintf("Статус: %s", status),
	}
	if msg.Text != "" {
		parts = append(parts, "Текст: "+truncateCommunityBanText(msg.Text))
	}
	if msg.Quote != nil && msg.Quote.Text != "" {
		parts = append(parts, "Цитата: "+truncateCommunityBanText(msg.Quote.Text))
	}
	return strings.Join(parts, "\n")
}

func buildCommunityBanFinalReport(caseItem *storage.ModerationCase, finalStatus string) string {
	return strings.Join([]string{
		"community-ban детект",
		fmt.Sprintf("Чат ID: %d", caseItem.ChatID),
		fmt.Sprintf("Сообщение ID: %d", caseItem.SpamMessageID),
		fmt.Sprintf("Пользователь ID: %d", caseItem.SuspectUserID),
		fmt.Sprintf("Статус: %s", finalStatus),
	}, "\n")
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

func communityBanChatName(chat *tele.Chat) string {
	if chat == nil {
		return "unknown"
	}
	if chat.Title != "" {
		return chat.Title
	}
	if chat.Username != "" {
		return "@" + chat.Username
	}
	return strconv.FormatInt(chat.ID, 10)
}
