-- Миграция 005: per-chat TTL автоудаления спам-сообщений
ALTER TABLE chat_config ADD COLUMN spam_delete_ttl_sec INTEGER NOT NULL DEFAULT 0;
