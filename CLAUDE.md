# swim-bot Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-02-15

## Active Technologies
- Go 1.22+ (go.mod: 1.25.6) + gopkg.in/telebot.v3 v3.3.8, modernc.org/sqlite v1.45.0 (002-smart-kick)
- SQLite с WAL mode, PRAGMA foreign_keys=ON, busy_timeout=5000 (002-smart-kick)
- Go 1.25.6 (go.mod) + gopkg.in/telebot.v3 v3.3.8, modernc.org/sqlite v1.45.0 (003-bot-hardening)
- SQLite (WAL mode, busy_timeout=5000, foreign_keys=ON) (003-bot-hardening)

- Go 1.22+ + gopkg.in/telebot.v3 (Telegram Bot API), modernc.org/sqlite (SQLite driver) (001-anti-spam-bot)

## Project Structure

```text
src/
tests/
```

## Commands

# Add commands for Go 1.22+

## Code Style

Go 1.22+: Follow standard conventions

## Recent Changes
- 004-cleanup-legacy: Added Go 1.25.6 (go.mod) + gopkg.in/telebot.v3 v3.3.8, modernc.org/sqlite v1.45.0
- 003-bot-hardening: Added Go 1.25.6 (go.mod) + gopkg.in/telebot.v3 v3.3.8, modernc.org/sqlite v1.45.0
- 003-bot-hardening: Added Go 1.22+ (go.mod: 1.25.6) + gopkg.in/telebot.v3 v3.3.8, modernc.org/sqlite v1.45.0


<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->


## Скиллы проекта

Spec-kit скиллы:
/home/spike/hobby/swim-bot/.claude/skills/
- speckit-analyze
- speckit-checklist
- speckit-clarify
- speckit-constitution
- speckit-implement
- speckit-plan
- speckit-specify
- speckit-tasks
- speckit-taskstoissues

## Деплой

Есть деплой на "ssh spike@datspike.xyz -p 51022"
