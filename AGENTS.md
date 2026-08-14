# Repo guidance

## Что это за репозиторий
- `swim-bot` — Telegram anti-spam bot на Go.
- Runtime stack: Go `1.25.6`, `gopkg.in/telebot.v4`, SQLite через `modernc.org/sqlite`.
- Бот работает через webhook HTTP endpoint, а не через long polling.
- SQLite schema migrates on startup from embedded SQL in `internal/storage/migrations`.

## Как ориентироваться
- `cmd/swim-bot` — entrypoint и lifecycle.
- `internal/bot` — handlers, middleware, rate limiting, retry, spam actions.
- `internal/storage` — SQLite access, migrations, persistence-backed limits and bans.
- `internal/config` — env-based config loading.
- `deploy` — reference service and reverse-proxy configs for production.
- `.specify` и `.claude/skills` — planning/spec artifacts; не тащить их в обычные кодовые правки без явной задачи.

## Базовый workflow
- Перед правками читай существующий код и держи изменения минимальными по scope.
- После backend-изменений обычно достаточно прогнать `go test ./...`.
- Если правка затрагивает стиль, логирование или error handling, дополнительно прогоняй `golangci-lint run`.
- Используй структурированные `slog`-логи; не добавляй `fmt.Print*` в runtime-код.
- Учитывай, что миграции применяются при старте процесса: изменение SQL и storage-кода требует проверки startup path, а не только unit tests.
- Для новых commit message используй Conventional Commits: `type(scope): summary`, например `fix(bot): skip repeated restrict` или `docs(readme): document setup`.

## Конфигурация и секреты
- Локальный пример env лежит в `.env.example`.
- Обязательные переменные: `TELEGRAM_TOKEN`, `WEBHOOK_URL`.
- Основные runtime knobs: `WEBHOOK_SECRET`, `PORT`, `LOG_LEVEL`, `DB_PATH`, `MAX_MESSAGE_AGE_SEC`.
- Не коммить реальные `.env`, токены, production DB и любые секреты из сервера.

## Deploy reality
- Production target verified on 2026-04-09: `ssh spike@datspike.xyz -p 51022`.
- На сервере бот запущен как systemd unit `swim-bot.service`.
- Runtime path on server: `/opt/swim-bot`.
- Systemd starts `/opt/swim-bot/swim-bot` with `EnvironmentFile=/opt/swim-bot/.env`; SQLite DB currently lives at `/opt/swim-bot/swim-bot.db`.
- Reverse proxy in repo is only reference material. Actual server state verified on 2026-04-09 uses Nginx on `datspike.xyz` over HTTPS and proxies `/swim-bot/webhook` to local port `8080`.
- Default deploy model remains binary rollout to `/opt/swim-bot` with service restart. GitHub Actions automation may exist only when explicitly introduced and should preserve the same production layout.

## Ожидания от deploy-related changes
- Если меняешь webhook path, port, service unit or proxy behavior, sync code assumptions with `deploy/swim-bot.service` and the proxy config in `deploy`.
- Не меняй production service layout, `.env`, DB path or server-side proxy assumptions без явной задачи: это high-impact operational scope.
- For deploy verification, prefer checking `systemctl status swim-bot`, `systemctl cat swim-bot`, `journalctl -u swim-bot`, and contents of `/opt/swim-bot` over guessing.

## Ожидания от изменений
- Preserve current architecture: thin `cmd/`, domain logic in `internal/bot`, persistence in `internal/storage`.
- Keep tests close to touched packages; add or update tests when changing spam detection, rate limiting, retry, or migration behavior.
- Prefer focused edits over broad refactors unless the task explicitly asks for cleanup.
