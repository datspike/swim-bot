-- Миграция 006: правила автоудаления сообщений от bot-аккаунтов
CREATE TABLE IF NOT EXISTS bot_delete_rule (
    chat_id      INTEGER NOT NULL,
    bot_username TEXT    NOT NULL,
    ttl_sec      INTEGER NOT NULL,
    created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (chat_id, bot_username),
    FOREIGN KEY (chat_id) REFERENCES chat_config(chat_id)
);

CREATE INDEX IF NOT EXISTS idx_bot_delete_rule_chat_id
    ON bot_delete_rule(chat_id);
