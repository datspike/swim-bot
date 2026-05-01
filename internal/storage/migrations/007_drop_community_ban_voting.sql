-- Миграция 007: удаление устаревшей ветки community-ban с голосованием
-- Runtime использует автоматический ban цитатного спама без moderation_case/vote.

DROP TABLE IF EXISTS moderation_vote;
DROP TABLE IF EXISTS moderation_case;
