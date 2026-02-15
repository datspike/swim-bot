-- Миграция 002: умный кик с rate limiting
-- Расширяет chat_config настройками лимитов, добавляет spam_counter

-- расширение chat_config для настроек rate limiting и стикерпака
ALTER TABLE chat_config ADD COLUMN tracked_sticker_pack TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_config ADD COLUMN daily_limit INTEGER NOT NULL DEFAULT 4;
ALTER TABLE chat_config ADD COLUMN reactive_limit INTEGER NOT NULL DEFAULT 1;
ALTER TABLE chat_config ADD COLUMN reactive_window_min INTEGER NOT NULL DEFAULT 15;
ALTER TABLE chat_config ADD COLUMN spam_density_threshold INTEGER NOT NULL DEFAULT 3;
ALTER TABLE chat_config ADD COLUMN spam_density_window_min INTEGER NOT NULL DEFAULT 5;

-- расширение action_log полями контекста и типа действия
ALTER TABLE action_log ADD COLUMN context INTEGER NOT NULL DEFAULT 0;
ALTER TABLE action_log ADD COLUMN action INTEGER NOT NULL DEFAULT 0;

-- счётчик спам-сообщений per-user per-chat per-day
CREATE TABLE IF NOT EXISTS spam_counter (
    chat_id         INTEGER NOT NULL,
    user_id         INTEGER NOT NULL,
    date            TEXT    NOT NULL,
    count           INTEGER NOT NULL DEFAULT 0,
    effective_limit INTEGER NOT NULL DEFAULT 4,
    kicked          INTEGER NOT NULL DEFAULT 0,
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (chat_id, user_id, date),
    FOREIGN KEY (chat_id) REFERENCES chat_config(chat_id)
);

-- составной индекс для запросов контекста (покрывает оба отдельных)
CREATE INDEX IF NOT EXISTS idx_action_log_chat_created
    ON action_log(chat_id, created_at);

-- удаление старых индексов (составной их покрывает)
DROP INDEX IF EXISTS idx_action_log_chat_id;
DROP INDEX IF EXISTS idx_action_log_created_at;
