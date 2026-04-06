-- Миграция 004: community ban для цитатного спама
-- Переключатели per-chat и хранение кейсов/голосов

ALTER TABLE chat_config ADD COLUMN community_ban_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_config ADD COLUMN spam_log_chat_id INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS moderation_case (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id               INTEGER NOT NULL,
    spam_message_id       INTEGER NOT NULL,
    suspect_user_id       INTEGER NOT NULL,
    bot_reply_message_id  INTEGER,
    log_chat_id           INTEGER NOT NULL DEFAULT 0,
    log_report_message_id INTEGER,
    status                TEXT    NOT NULL DEFAULT 'open',
    created_at            TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at            TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE (chat_id, spam_message_id)
);

CREATE TABLE IF NOT EXISTS moderation_vote (
    case_id     INTEGER NOT NULL,
    voter_user_id INTEGER NOT NULL,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (case_id, voter_user_id),
    FOREIGN KEY (case_id) REFERENCES moderation_case(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_moderation_case_chat_status
    ON moderation_case(chat_id, status, created_at);

CREATE INDEX IF NOT EXISTS idx_moderation_vote_case_id
    ON moderation_vote(case_id);
