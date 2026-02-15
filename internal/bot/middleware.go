// Package bot содержит логику Telegram бота.
package bot

import (
	"log/slog"

	tele "gopkg.in/telebot.v3"
)

// AdminOnly middleware проверяет, что отправитель является администратором указанного чата.
// chatID извлекается из контекста через ключ "target_chat_id" (int64).
// Если chatID не указан, используется чат из сообщения.
func AdminOnly(logger *slog.Logger) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			// определяем целевой чат
			var chatID int64
			if id, ok := c.Get("target_chat_id").(int64); ok {
				chatID = id
			} else if c.Chat() != nil {
				chatID = c.Chat().ID
			} else {
				logger.Warn("admin check failed", "reason", "no_chat_id", "user_id", c.Sender().ID)
				return c.Send("Не удалось определить чат для проверки прав.")
			}

			// получаем информацию о пользователе в чате
			member, err := c.Bot().ChatMemberOf(&tele.Chat{ID: chatID}, c.Sender())
			if err != nil {
				logger.Warn("admin check failed", "reason", "api_error", "chat_id", chatID, "user_id", c.Sender().ID, "error", err)
				return c.Send("Не удалось проверить права. Возможно, я не добавлен в этот чат.")
			}

			// проверяем роль
			if member.Role != tele.Administrator && member.Role != tele.Creator {
				logger.Info("admin check failed", "reason", "not_admin", "chat_id", chatID, "user_id", c.Sender().ID, "role", member.Role)
				return c.Send("Ты не администратор этого чата.")
			}

			return next(c)
		}
	}
}

// PrivateOnly middleware пропускает только сообщения из приватных чатов.
func PrivateOnly() tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			if c.Chat() == nil || c.Chat().Type != tele.ChatPrivate {
				// молча игнорируем сообщения из групп
				return nil
			}
			return next(c)
		}
	}
}
