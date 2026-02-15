-- Миграция 003: hardening — тестовый режим, ring buffer параметры
-- Расширяет chat_config для dual code path (новая/legacy логика)

-- тестовый режим per-chat (FR-014, FR-020)
ALTER TABLE chat_config ADD COLUMN test_mode INTEGER NOT NULL DEFAULT 0;

-- параметры ring buffer для реактивного контекста (FR-010b)
-- M — размер скользящего окна, N — порог спам-событий
ALTER TABLE chat_config ADD COLUMN ring_buffer_size INTEGER NOT NULL DEFAULT 20;
ALTER TABLE chat_config ADD COLUMN ring_buffer_threshold INTEGER NOT NULL DEFAULT 2;
