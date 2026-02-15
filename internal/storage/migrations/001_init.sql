-- Миграция 001: начальная схема для Anti-Spam Kick Bot
-- Создаёт таблицы chat_config и action_log

CREATE TABLE IF NOT EXISTS chat_config (
    chat_id         INTEGER PRIMARY KEY,
    tracked_bot     TEXT    NOT NULL DEFAULT '',
    sticker_file_id TEXT    NOT NULL DEFAULT '',
    is_active       INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS action_log (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id          INTEGER NOT NULL,
    user_id          INTEGER NOT NULL,
    spam_message_id  INTEGER NOT NULL,
    reply_message_id INTEGER,
    created_at       TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (chat_id) REFERENCES chat_config(chat_id)
);

CREATE INDEX IF NOT EXISTS idx_action_log_chat_id
    ON action_log(chat_id);

CREATE INDEX IF NOT EXISTS idx_action_log_created_at
    ON action_log(created_at);
