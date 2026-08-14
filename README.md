# swim-bot

`swim-bot` — Telegram anti-spam бот для групповых чатов. Он работает через webhook, отслеживает сообщения, отправленные через заданного inline-бота (`via_bot`), ведёт дневные счётчики нарушений и ограничивает пользователя после исчерпания лимита. Дополнительно есть режим community-ban для спама цитатами из каналов и per-chat автоудаление спам-сообщений.

## Возможности

- Webhook-приём Telegram updates без long polling.
- Настройка отслеживаемого inline-бота отдельно для каждого чата.
- Дневной лимит спам-срабатываний per-user/per-chat.
- `restrict` пользователя до конца UTC-дня после исчерпания лимита.
- Реактивный штраф через ring buffer: если в чате уже была волна спама от других пользователей, новая попытка может списывать дополнительный лимит.
- Тестовый режим для проверки правил без скрытого поведения.
- Community-ban для сообщений с цитатой из каналов: удаление сообщения, бан пользователя, короткое уведомление в чат и опциональный лог в отдельный чат.
- Per-chat автоудаление сообщений, распознанных как spam via tracked bot.
- Per-chat автоудаление прямых сообщений от заданных Telegram bot-аккаунтов по TTL.
- Защита от webhook backlog: старые сообщения после перезапуска пропускаются.
- SQLite-хранилище с миграциями при старте.
- JSON-логи через `log/slog`.

## Стек

- Go `1.25.6`
- `gopkg.in/telebot.v4`
- SQLite через `modernc.org/sqlite`
- systemd для production-процесса
- GitHub Actions для сборки и деплоя бинарника

## Быстрый старт локально

1. Установить Go версии из `go.mod`.
2. Создать Telegram-бота через `@BotFather` и получить токен.
3. Подготовить окружение:

```bash
cp .env.example .env
```

4. Заполнить `.env`:

```env
TELEGRAM_TOKEN=123456789:token
WEBHOOK_URL=https://example.com/swim-bot/webhook
WEBHOOK_SECRET=your-random-secret-string
PORT=8080
LOG_LEVEL=info
DB_PATH=./swim-bot.db
MAX_MESSAGE_AGE_SEC=30
```

5. Запустить тесты и сборку:

```bash
go test ./...
go build -o swim-bot ./cmd/swim-bot
```

6. Запустить бота:

```bash
set -a
source .env
set +a
./swim-bot
```

Для локальной проверки webhook нужен публичный HTTPS URL до локального порта `PORT` — например через туннель или reverse proxy.

## Конфигурация окружения

| Переменная | Обязательная | Значение по умолчанию | Описание |
|---|---:|---|---|
| `TELEGRAM_TOKEN` | да | — | Токен Telegram-бота от `@BotFather`. |
| `WEBHOOK_URL` | да | — | Публичный HTTPS URL, который Telegram вызывает для updates. |
| `WEBHOOK_SECRET` | нет | пусто | Секрет для проверки заголовка `X-Telegram-Bot-Api-Secret-Token`. Если задан, запросы без корректного заголовка отклоняются. |
| `PORT` | нет | `8080` | Локальный HTTP-порт webhook listener. |
| `LOG_LEVEL` | нет | `info` | Уровень логов: `debug`, `info`, `warn`, `error`. |
| `DB_PATH` | нет | `data.db` | Путь к SQLite базе данных. |
| `MAX_MESSAGE_AGE_SEC` | нет | `30` | Максимальный возраст сообщения для обработки. Более старые updates пропускаются после перезапуска. |

Автоудаление спам-сообщений и прямых сообщений от заданных bot-аккаунтов настраивается не через env, а отдельно для каждого чата командами `/setspamdelete` и `/setbotdelete`.

## Настройка Telegram-чата

Бота нужно добавить в целевой групповой чат как администратора. Для полного набора функций нужны права:

- читать сообщения и получать updates;
- удалять сообщения;
- ограничивать пользователей;
- банить пользователей, если используется community-ban.

Все команды настройки выполняются в личном чате с ботом. Для команд нужен числовой `chat_id` целевого чата. Его можно получить любым привычным способом, например через `@raw_data_bot`.

Минимальная настройка spam via bot:

```text
/setbot <chat_id> @tracked_inline_bot
```

Пример:

```text
/setbot -100123456789 @mlversebot
```

После этого бот активирует чат и начнёт считать спамом сообщения, у которых `message.via_bot.username` совпадает с настроенным username.

## Команды бота

Команды доступны в личном чате с ботом. Для указанного `chat_id` отправитель должен быть администратором или владельцем этого чата.

| Команда | Назначение |
|---|---|
| `/start` | Краткая справка и быстрый старт. |
| `/help` | Полная справка по командам. |
| `/setbot <chat_id> @username` | Назначить inline-бота, через которого сообщения считаются спамом. Username нормализуется: `@` убирается, регистр приводится к нижнему. |
| `/setlimits <chat_id> [daily=N] [rb_size=N] [rb_threshold=N] [rb_steal=on\|off]` | Настроить дневной лимит и параметры reactive ring buffer. |
| `/getlimits <chat_id>` | Показать текущие лимиты и статус reactive-штрафа. |
| `/setspamdelete <chat_id> <seconds>` | Настроить автоудаление исходных spam via bot сообщений. `0` выключает автоудаление. |
| `/setbotdelete <chat_id> <bot_username> <seconds>` | Настроить автоудаление прямых сообщений от указанного Telegram bot-аккаунта. `0` удаляет правило. |
| `/delbotdelete <chat_id> <bot_username>` | Удалить правило автоудаления прямых сообщений от bot-аккаунта. |
| `/listbotdelete <chat_id>` | Показать правила автоудаления прямых bot-сообщений для чата. |
| `/testmode <chat_id> on\|off` | Включить или выключить тестовый режим. |
| `/resetcounters <chat_id> [force=on]` | Сбросить дневные spam-счётчики и снять выданные ботом дневные ограничения, если записи счётчиков ещё существуют. Без `force=on` доступно только в test mode. |
| `/stats <chat_id>` | Показать число срабатываний, последний триггер, tracked bot и активность. |
| `/setcommunityban <chat_id> on\|off` | Включить или выключить community-ban для цитатного спама. |
| `/setspamlog <chat_id> <target_chat_id>` | Назначить чат для логов community-ban. |
| `/communitybanstatus <chat_id>` | Показать статус community-ban и текущий log-chat. |

Примеры:

```text
/setlimits -100123456789 daily=6 rb_size=30 rb_threshold=3 rb_steal=on
/getlimits -100123456789
/setspamdelete -100123456789 60
/setbotdelete -100123456789 clown_alert_bot 60
/listbotdelete -100123456789
/testmode -100123456789 on
/resetcounters -100123456789 force=on
/setcommunityban -100123456789 on
/setspamlog -100123456789 -100987654321
```

## Как работает spam via bot

1. Для каждого группового сообщения бот загружает `chat_config` из SQLite.
2. Если чат не настроен или неактивен, сообщение игнорируется.
3. Старые сообщения старше `MAX_MESSAGE_AGE_SEC` игнорируются.
4. Сообщение считается спамом, если:
   - есть отправитель;
   - есть `via_bot`;
   - username `via_bot` совпадает с configured tracked bot.
5. Администраторы и владельцы чата не проходят spam-обработку.
6. Для обычного пользователя обновляется дневной spam counter:
   - по умолчанию лимит `daily_limit = 4`;
   - до исчерпания лимита бот отправляет предупреждение;
   - после исчерпания лимита бот ограничивает `can_send_other_messages` до конца UTC-дня.
7. `/resetcounters <chat_id> [force=on]` перед удалением дневных счётчиков ищет пользователей с `kicked = 1` за текущую UTC-дату и снимает выданные ботом дневные ограничения; если счётчики уже удалены, команда не восстанавливает кандидатов из `action_log`.
8. Каждое срабатывание пишется в `action_log`.
9. Если для чата включён `spam_delete_ttl_sec`, исходное spam-сообщение удаляется через заданное число секунд.

Сообщения с предупреждением вида `Вы можете поплавать ещё ...` автоудаляются через 1 минуту. Финальные restrict-сообщения не удаляются этим механизмом.

## Автоудаление прямых сообщений от bot-аккаунтов

Механизм `/setbotdelete` не связан с `tracked_bot` и `via_bot`. Он применяется только к сообщениям, где прямой отправитель — Telegram bot-аккаунт (`message.sender.is_bot`) и его username совпадает с правилом для чата.

Пример:

```text
/setbotdelete -100123456789 clown_alert_bot 60
```

После этого прямые сообщения от `@clown_alert_bot` в чате `-100123456789` будут планироваться на удаление через 60 секунд. Запланированные удаления хранятся только в памяти процесса и могут потеряться при рестарте до истечения TTL.

## Reactive ring buffer

Reactive-режим нужен для волн спама, когда несколько пользователей подряд отправляют сообщения через tracked bot.

Настройки per-chat:

- `daily` — дневной лимит попыток, по умолчанию `4`;
- `rb_size` — размер скользящего окна сообщений, по умолчанию `20`;
- `rb_threshold` — сколько spam-событий от других пользователей в окне включает reactive-контекст, по умолчанию `2`;
- `rb_steal=off` — отключает reactive-штраф, устанавливая `rb_threshold = 0`.

В reactive-контексте новая spam-попытка списывает обычную попытку и дополнительно «ворует» ещё одну, если лимит ещё не исчерпан.

## Community-ban

Community-ban — отдельный механизм для подозрительных сообщений с цитатами из каналов. Сообщение считается кандидатом, если оно:

- отправлено не ботом;
- не является `via_bot` и не является forward;
- содержит `ExternalReply` и `Quote`;
- origin chat цитаты — канал.

Если community-ban включён и отправитель не администратор:

1. исходное сообщение удаляется;
2. пользователь банится;
3. в чат отправляется короткое уведомление, которое удаляется через 1 минуту;
4. если настроен spam-log, туда отправляется отчёт.

Включение:

```text
/setcommunityban <chat_id> on
/setspamlog <chat_id> <target_chat_id>
```

## Хранение данных

SQLite база задаётся через `DB_PATH`. При открытии соединения добавляются PRAGMA:

- `busy_timeout(5000)`;
- `foreign_keys(ON)`;
- `journal_mode(WAL)`.

Миграции встроены через `go:embed` и применяются при старте по `PRAGMA user_version`.

Основные таблицы:

| Таблица | Назначение |
|---|---|
| `chat_config` | Настройки чата: tracked bot, активность, лимиты, test mode, community-ban, spam-log, TTL автоудаления. |
| `action_log` | История spam-срабатываний. |
| `spam_counter` | Дневные счётчики per-chat/per-user/per-date. |
| `bot_delete_rule` | Per-chat правила автоудаления прямых сообщений от заданных Telegram bot-аккаунтов. |

## Архитектура проекта

```text
cmd/swim-bot/              entrypoint, config loading, storage init, migrations, lifecycle
internal/config/           загрузка env-конфигурации
internal/bot/              Telegram handlers, webhook poller, spam logic, retry, ring buffer
internal/storage/          SQLite access, migrations, persisted counters/configs
internal/storage/migrations встроенные SQL-миграции
deploy/                    reference systemd/reverse-proxy/deploy docs
.github/workflows/         GitHub Actions deploy
```

Ключевой поток выполнения:

```text
Telegram webhook
  -> safeWebhook.ServeHTTP
  -> telebot handler
  -> handleSpamDetection
  -> GetChatConfig
  -> via_bot/community-ban detection
  -> processSpam или handleCommunityBanDetection
  -> Telegram API side effects
  -> SQLite action/counter/config updates
```

`safeWebhook` повторяет контракт `tele.Webhook`, но избегает двойного закрытия stop-канала при shutdown в используемой версии `telebot`.

## Тесты и проверки

Основная проверка:

```bash
go test ./...
```

Сборка production-бинарника для Linux amd64:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o dist/swim-bot ./cmd/swim-bot
```

Если изменение затрагивает стиль, логирование или обработку ошибок, дополнительно запусти:

```bash
golangci-lint run
```

## Production deploy

Reference systemd unit находится в [`deploy/swim-bot.service`](deploy/swim-bot.service). Он ожидает такую layout-схему:

```text
/opt/swim-bot/
  swim-bot       бинарник
  .env           production env, не хранится в git
  swim-bot.db    SQLite база
  backups/       резервные копии бинарника при deploy
```

Сервис запускает `/opt/swim-bot/swim-bot` с `EnvironmentFile=/opt/swim-bot/.env` и рабочей директорией `/opt/swim-bot`.

GitHub Actions workflow [`deploy.yml`](.github/workflows/deploy.yml):

1. запускает `go test ./...`;
2. собирает `linux/amd64` бинарник;
3. загружает только бинарник на сервер;
4. сохраняет резервную копию текущего бинарника;
5. заменяет `/opt/swim-bot/swim-bot`;
6. перезапускает `swim-bot.service`;
7. проверяет статус сервиса и последние логи.

Workflow не изменяет production `.env`, SQLite базу, systemd unit и reverse proxy config.

Reverse proxy должен проксировать публичный webhook path из `WEBHOOK_URL` на локальный `PORT`. В `deploy/caddy-swim-bot.conf` есть reference-конфигурация для Caddy; фактическая production-конфигурация может отличаться.

## Rollback

Если деплой через GitHub Actions уже сделал backup, откат на сервере:

```bash
cp /opt/swim-bot/backups/swim-bot-<timestamp> /opt/swim-bot/swim-bot
sudo systemctl restart swim-bot.service
```

## Безопасность и секреты

- Не коммить `.env`, реальные токены, SQLite базы и production-секреты.
- `WEBHOOK_SECRET` желательно задавать всегда: Telegram будет отправлять его в `X-Telegram-Bot-Api-Secret-Token`, а бот отклонит запросы с неверным значением.
- Боту нужны только те административные права, которые соответствуют включённым функциям: удаление сообщений, restrict и ban.
- Все команды настройки проверяют, что отправитель является администратором или владельцем целевого чата.
