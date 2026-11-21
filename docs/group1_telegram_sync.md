# Группа 1 (2 человека): Внедрение чтения диалогов и сообщений из Telegram

## Цель группы
Перевести работу с Telegram из консольной утилиты (`cmd/telegram-dev`) в полноценный серверный модуль синхронизации: подключённые рабочие аккаунты из таблицы `accounts` должны автоматически и по ручному триггеру выгружать личные диалоги и историю сообщений в PostgreSQL, обновляя состояние чатов и дневную статистику.

**Важно (база из коммита `4c4b515`):** в `internal/telegram/client.go` уже реализованы кроссплатформенный HTTP/HTTPS/SOCKS5 прокси-дайлер (`createProxyDialer` + `bufferedConn`), `tg.UpdateDispatcher` и штатный QR-логин через `gotd/td/telegram/auth/qrlogin` с обработкой 2FA. Группе 1 нужно переиспользовать этот транспорт в серверном клиенте, а не переписывать его с нуля.

---

## Разделение работы внутри группы (2 разработчика)

| Роль | Зона ответственности | Основные файлы |
| :--- | :--- | :--- |
| **Участник 1A — MTProto-транспорт, прокси и сессии в БД** | Секция `TelegramConfig` в конфиге (включая `TELEGRAM_PROXY`), адаптер `session.Storage` поверх `accounts.session` в PostgreSQL, переиспользование `createProxyDialer`, пагинация `MessagesGetDialogs` (личные `PeerUser`), инкрементальная выгрузка `MessagesGetHistory`, маппинг `Direction`/`SentAt`/`MediaType`, обработка `FLOOD_WAIT` и статусов сессии. | `internal/config/config.go`, `internal/telegram/*`, `internal/storage/postgres/account.go` (`UpdateSession`, `UpdateLastActivity`), `internal/usecase/interfaces.go` |
| **Участник 1B — Оркестрация синхронизации, воркер и API** | Сервис `SyncUsecase` (связка `Contact` → `Chat` → `Message` → `AccountDailyStats`), обновление `last_message_at` / `awaiting_reply_since` / `unread_count`, фоновый воркер с изоляцией ошибок и graceful shutdown, HTTP-ручка `POST /accounts/:id/sync` (и при необходимости QR-авторизация аккаунта через API), unit-тесты. | `internal/usecase/sync.go`, `internal/usecase/sync_test.go`, `internal/storage/postgres/chat.go` (поиск/обновление чата), `internal/storage/postgres/message.go` (последний `telegram_message_id`), `internal/handler/handler.go`, `cmd/app/main.go` |

### Точки синхронизации с другими группами
- **Не трогать:** `internal/ai/`, `internal/usecase/chat_summary.go`, `internal/usecase/alert.go`, `internal/usecase/chat_analytics.go` — это зона Группы 2.
- **Контракт для Группы 2:** после сохранения новых сообщений в чате `SyncUsecase` вызывает опциональный коллбек `OnChatUpdated(ctx context.Context, companyID, chatID int64)`, чтобы Группа 2 могла подключить асинхронный пересчёт AI-саммари без конфликтов.
- **Номер миграции:** если потребуется миграция `000009` (например, индекс `messages(chat_id, telegram_message_id DESC)`), согласуйте её номер с Группой 2.

---

## План реализации по шагам (сначала короткий RFC)

Перед написанием кода зафиксируйте в `docs/rfc/01-telegram-sync.md` 5 блоков по методике курса:
1. **Фича:** подключение сохранённой сессии рабочего аккаунта (включая работу через прокси) и автоматическая подгрузка личных переписок в систему.
2. **Бизнес-требования:** синхронизируются только личные чаты (`PeerUser`), группы/каналы/боты игнорируются; повторный прогон не создаёт дублей; падение одного аккаунта не останавливает остальные.
3. **Технические требования:** хранилище сессий поверх `accounts.session`, поддержка `TELEGRAM_PROXY` (SOCKS5/HTTP/HTTPS), инкрементальная выгрузка после `max(telegram_message_id)`, таймауты на RPC-вызовы, обработка `FLOOD_WAIT`, graceful shutdown.
4. **Контракты:** `POST /accounts/:id/sync` (ручной запуск синхронизации с возвратом числа обработанных чатов и новых сообщений).
5. **Схема БД:** переиспользуются `accounts`, `contacts`, `chats`, `messages`, `account_daily_stats`; при необходимости добавляется индекс под быстрый поиск последнего `telegram_message_id`.

---

## Готовый промпт для кодинг-агента (Группа 1)

```text
Изучи текущую реализацию Telegram-клиента в internal/telegram/client.go (включая уже готовые createProxyDialer, bufferedConn и QR-авторизацию через gotd/td/telegram/auth/qrlogin), CLI в cmd/telegram-dev/main.go, конфиг в internal/config/config.go, доменные модели Account, Contact, Chat, Message и AccountDailyStats в internal/domain/, хранилища в internal/storage/postgres/ и сборку в cmd/app/main.go.

Сейчас клиент в internal/telegram/client.go работает только через локальный файл session.FileStorage для CLI и не связан с основным HTTP-сервером. Также в fetchDialogs (internal/telegram/client.go) есть мелкий баг: ключи групп сохраняются как "chat_<id>", а ищутся как "group_<id>", плюс дублируется код для MessagesDialogs и MessagesDialogsSlice — исправь это заодно.

Нужно реализовать полноценное чтение личных диалогов и истории сообщений с подключённых Telegram-аккаунтов и сохранение их в PostgreSQL, не ломая чистую архитектуру (handler -> usecase -> storage) и существующие тесты:

1. Конфигурация (internal/config/config.go):
   - Добавь секцию TelegramConfig: TELEGRAM_API_ID, TELEGRAM_API_HASH, TELEGRAM_PROXY (с учётом USE_PROXY / TELEGRAM_NO_PROXY), TELEGRAM_SYNC_ENABLED, TELEGRAM_SYNC_INTERVAL (по умолчанию 60s), TELEGRAM_SYNC_BATCH_LIMIT (по умолчанию 100).
   - Если TELEGRAM_API_ID или TELEGRAM_API_HASH не заданы, сервер не должен падать при старте — фоновая синхронизация отключается с понятным логом, а ручной вызов POST /accounts/:id/sync возвращает 503 Service Unavailable.

2. Хранение сессии в БД и MTProto-адаптер (internal/telegram/):
   - Реализуй адаптер session.Storage (из github.com/gotd/td/session) поверх поля accounts.session в PostgreSQL (добавь методы UpdateSession и UpdateLastActivity в AccountRepository и AccountStorage), сохранив совместимость FileStorage для cmd/telegram-dev.
   - Переиспользуй существующий createProxyDialer для всех серверных MTProto-подключений.
   - Определи в internal/usecase/interfaces.go абстрактный интерфейс TelegramSyncClient, чтобы SyncUsecase не зависел напрямую от gotd/td и тестировался на моках.
   - Реализуй чтение личных диалогов (PeerUser, исключая группы, каналы и ботов) с полной пагинацией MessagesGetDialogs (OffsetDate, OffsetID, OffsetPeer) и контекстными таймаутами на каждый RPC-вызов.
   - Реализуй инкрементальное чтение истории по каждому диалогу через MessagesGetHistory с передачей MinID (последнего сохранённого telegram_message_id для этого чата), чтобы догружать только новые сообщения.
   - Извлекай для каждого сообщения: TelegramMessageID, Direction ("in" или "out" по признаку Out), Text, SentAt (UTC) и MediaType ("photo", "document", "voice" без скачивания самих файлов).
   - Обрабатывай ошибки MTProto: при FLOOD_WAIT делай паузу с учётом ctx.Done(), при отозванной/невалидной сессии переводи аккаунт в статус disconnected, при блокировке — в banned.

3. Сервис синхронизации (internal/usecase/sync.go):
   - Реализуй SyncUsecase с методом SyncAccount(ctx, companyID, accountID) (возвращает статистику: обработано чатов, загружено новых сообщений).
   - Для каждого личного диалога:
     a) Находит или создаёт собеседника через ContactRepository (FindOrCreateByTelegramID).
     b) Находит или создаёт диалог в ChatRepository по паре (account_id, contact_id).
     c) Загружает только новые сообщения после max(telegram_message_id) и сохраняет через MessageRepository.Create (защита ON CONFLICT (chat_id, telegram_message_id) DO NOTHING).
     d) Обновляет метаданные чата (last_message_at, unread_count и awaiting_reply_since: если последнее сообщение входящее — фиксируется время ожидания, если исходящее — сбрасывается в nil).
     e) Инкрементирует дневную статистику через AccountDailyStatsRepository.IncrementCounters (только для реально новых, не дублированных сообщений) и обновляет accounts.last_activity_at.
   - Предусмотри опциональный неблокирующий хук OnChatUpdated(ctx, companyID, chatID) для триггера AI-саммаризации (Группа 2).

4. Фоновый воркер и HTTP API (cmd/app/main.go, internal/handler/handler.go):
   - Добавь фоновый воркер в cmd/app/main.go, который периодически синхронизирует активные аккаунты с непустой сессией; ошибка одного аккаунта не должна останавливать остальные и HTTP-сервер.
   - Привяжи остановку воркера к общему контексту graceful shutdown (SIGINT/SIGTERM).
   - Добавь HTTP-роут POST /accounts/:id/sync (с проверкой companyID через parseCompanyID).

5. Тесты и проверки:
   - Покрой unit-тестами SyncUsecase (инкрементальная выгрузка, защита от дублей, обновление awaiting_reply_since, обработка ошибок сессии, изоляция по companyID) и исправленный fetchDialogs.
   - Прогони gofmt -w ., go vet ./..., go test -race ./... и go build ./.... Не меняй версии в go.mod и Dockerfile.
```

---

## Критерии приёмки (Definition of Done)
- `go test -race ./...`, `go vet ./...` и `go build ./...` проходят без ошибок.
- Исправлен баг с ключом `"chat_"`/`"group_"` в `fetchDialogs`, `cmd/telegram-dev` продолжает собираться и работать.
- При запуске сервера без `TELEGRAM_API_ID`/`TELEGRAM_API_HASH` сервер стартует штатно, логирует отключение синка, а `POST /accounts/:id/sync` возвращает `503`.
- При повторном запуске `SyncAccount` дубликаты в `messages` и двойной инкремент в `account_daily_stats` не происходят.
- При `SIGINT`/`SIGTERM` воркер корректно завершает текущую итерацию без утечки горутин.
