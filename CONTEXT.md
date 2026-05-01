# Swim Bot Context

Этот контекст фиксирует доменные термины Telegram anti-spam bot, чтобы архитектурные Module назывались языком проекта, а не техническими случайностями.

## Language

**Chat Configuration**:
Настройки swim-bot для одного Telegram-чата.
_Avoid_: settings row, config record

**Tracked Bot**:
Inline-бот, сообщения через которого считаются спам-сигналом.
_Avoid_: spam bot, via bot setting

**Spam Processing**:
Правила, которые превращают сообщение и состояние чата в предупреждение, ограничение, удаление или пропуск.
_Avoid_: handler flow, rate-limit code

**Reactive Context**:
Ситуация, когда в скользящем окне уже есть достаточно спам-сигналов от других пользователей.
_Avoid_: burst mode, group spam mode

**Community Ban**:
Автобан сообщения пользователя с подозрительной цитатой из канала.
_Avoid_: moderation voting, quote spam handler

**Bot Delete Rule**:
Правило автоудаления прямых сообщений от указанного bot-аккаунта в чате.
_Avoid_: bot cleanup setting, delete TTL row

**Runtime Storage**:
SQLite-хранилище после миграций и startup-обновлений, готовое к работе бота.
_Avoid_: database connection, migrated DB

**Webhook Transport**:
HTTP webhook-приёмник Telegram updates, подключаемый к telebot как poller.
_Avoid_: safe webhook hack, HTTP server code

## Relationships

- A **Chat Configuration** may define one **Tracked Bot**.
- A **Chat Configuration** may enable **Community Ban**.
- A **Chat Configuration** may contain zero or more **Bot Delete Rules**.
- **Spam Processing** uses **Chat Configuration** and message history to identify **Reactive Context**.
- **Runtime Storage** stores **Chat Configuration**, spam counters, action logs, and **Bot Delete Rules**.
- **Webhook Transport** delivers Telegram updates to **Spam Processing** through telebot handlers.

## Example dialogue

> **Dev:** "If **Community Ban** is on but no **Tracked Bot** is configured, should the chat still be active?"
> **Domain expert:** "Yes. **Community Ban** is its own rule in **Chat Configuration**, so **Spam Processing** must still inspect group messages."

## Flagged ambiguities

- "bot" can mean the swim-bot process, a **Tracked Bot**, or a direct bot sender covered by a **Bot Delete Rule**; use the specific term when changing rules.
- "database" can mean a raw SQLite connection or **Runtime Storage**; startup code should prefer **Runtime Storage** when migrations and startup updates are required.
