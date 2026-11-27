# TeleCorp Analytics

Аналитика отдельного чата: `GET /chats/:id/analytics` — счётчики сообщений, время ответа и активность по дням. [Описание API и правил расчёта](docs/chat-analytics.md).

Менеджер корпоративных Telegram-аккаунтов для малого и среднего бизнеса: контроль переписок и активности сотрудников без ручного просмотра чатов.

## Структура

```
cmd/app/main.go               точка входа: читает конфиг, подключает БД и запускает сервер
docs/                         требования и техническое задание проекта
migrations/                   SQL-миграции схемы базы данных
internal/config/              настройки из переменных окружения
internal/domain/              сущности и ошибки предметной области
internal/handler/             HTTP-слой: роуты, разбор запросов, ответы
internal/usecase/             бизнес-логика и интерфейсы хранилищ
internal/storage/postgres/    реализация работы с базой данных (PostgreSQL + sqlx)
```

## Как запустить

Понадобятся [Go 1.26+](https://go.dev/dl/) и [Docker](https://www.docker.com/products/docker-desktop/) для запуска в контейнере.

### Локально

```bash
go run ./cmd/app
```

### В Docker

```bash
docker build -t go-server:1 .
docker run --rm -p 8080:8080 go-server:1
```

Готовый образ без сборки:

```bash
docker run --rm -p 8080:8080 ghcr.io/kraconst/go-server:1
```

### Настройки

| Переменная    | По умолчанию       | Назначение                    |
|---------------|--------------------|-------------------------------|
| `HTTP_ADDR`   | `:8080`            | адрес, который слушает сервер |
| `DB_HOST`     | `localhost`        | хост PostgreSQL               |
| `DB_PORT`     | `5432`             | порт PostgreSQL               |
| `DB_USER`     | `telecorp`         | пользователь базы данных      |
| `DB_PASSWORD` | `telecorp_secret`  | пароль базы данных            |
| `DB_NAME`     | `telecorp_db`      | имя базы данных               |
| `DB_SSLMODE`  | `disable`          | режим SSL для подключения к БД|
| `AI_API_BASE_URL` | `https://api.openai.com/v1` | base URL OpenAI-совместимого API |
| `AI_API_KEY`  | пусто              | ключ AI; пусто — AI отключён, generate отвечает `503` |
| `AI_MODEL`    | —                  | модель AI; без неё AI отключён |
| `AI_TIMEOUT`  | `30s`              | таймаут запроса к AI (Go duration, не больше `5m`); `WriteTimeout` и ожидание HTTP-запросов при остановке = `AI_TIMEOUT + 5s` (плюс до 5s на фоновый воркер) |
| `AI_MAX_MESSAGES` | `50`           | максимум сообщений в контексте AI |
| `AI_MAX_CHARS` | `12000`           | максимум символов текста сообщений в контексте AI |

### База данных (PostgreSQL)

Запуск базы через Docker Compose:

```bash
docker compose up -d postgres
```

## API

| Метод | Путь             | Ответ                                         |
|-------|------------------|-----------------------------------------------|
| GET   | `/health`        | `200` — сервис жив                            |
| GET   | `/accounts`      | `200` — список аккаунтов                      |
| GET   | `/accounts/:id`  | `200` — аккаунт, `404` — нет такого, `400` — id не число |
| POST  | `/accounts`      | `201` — аккаунт создан, `400` — не хватает полей |

Пример запроса на создание:

```bash
curl -X POST http://localhost:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{"phone":"+79990000000","username":"@new","employee_name":"Ира"}'
```

В PowerShell вместо `curl` пишите `curl.exe`, а для POST удобнее `Invoke-RestMethod`:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/accounts -ContentType "application/json" -Body '{"phone":"+79990000000","username":"@new","employee_name":"Ира"}'
```

## Уведомления

| Метод | Путь | Ответ |
|-------|------|-------|
| GET | `/alerts` | `200` — активные уведомления компании, новые первыми; пустой список — `[]` |
| POST | `/alerts/:id/resolve` | `200` — обработано, `404` — уведомление не найдено в компании, `400` — некорректный ID |

Типы: `no_reply`, `complaint`, `refusal`, `negative`, `urgent`. Закрытие сохраняет
`resolved_at`; повторный POST успешен и не изменяет время первого закрытия.
После обработки уведомление исчезает из активной ленты, запись остаётся в БД.
Как и остальные текущие ручки, API пока использует компанию с ID 1; JWT и роли
будут добавлены отдельно. Чтение и закрытие в хранилище ограничены `company_id`.

Создание предусмотрено через `AlertUsecase.Create` для будущих обработчиков
событий Telegram и AI. Автоматическое обнаружение проблем и отправка во внешние
каналы в этой реализации не запускаются. Пока таблица `chats` разрабатывается
в другой ветке, `chat_id` хранится как положительный ID без внешнего ключа —
аналогично текущей таблице `messages`.

При первом запуске PostgreSQL с новым томом Compose применяет up-миграции
`000001`, `000002` и `000005`. Номера `000003` и `000004` заняты отдельными
ветками контактов/чатов и статистики. Для существующего тома выполните из корня
проекта (PowerShell):

```powershell
Get-Content -Raw migrations/000005_create_alerts.up.sql | docker compose exec -T postgres psql -U telecorp -d telecorp_db -v ON_ERROR_STOP=1
```

Если PostgreSQL недоступен, демонстрационная лента работает в памяти процесса:
уведомления можно закрыть, а при перезапуске сервера демонстрационные данные
восстанавливаются.

Проверки уведомлений:

```powershell
go test ./internal/usecase ./internal/handler
$env:TEST_POSTGRES_DSN = "postgres://telecorp:telecorp_secret@localhost:5432/telecorp_db?sslmode=disable"
go test ./internal/storage/postgres -run TestAlertPostgresLifecycle -v
```

Интеграционный тест создаёт отдельную временную схему и удаляет её после проверки;
нужны права на создание схем. Без `TEST_POSTGRES_DSN` он пропускается.

## Тестовое подключение Telegram

Отдельная команда `cmd/telegram-dev` подключает существующий аккаунт через
MTProto (`gotd/td`). HTTP-сервер и база данных для неё не нужны.
Ключи приложения получают в [API development tools](https://my.telegram.org).

Утилита автоматически загружает переменные из файла `.env` (если он есть), либо их можно задать вручную в интерактивном PowerShell:

```powershell
$env:TELEGRAM_API_ID = "123456"
$env:TELEGRAM_API_HASH = "ваш_api_hash"
$env:TELEGRAM_PHONE = "+79991234567"
$env:TELEGRAM_SESSION_PATH = "./var/telegram/account-a.session"
go run ./cmd/telegram-dev -limit 20
```

Пример переменных есть в `.env.example`. Переменные окружения процесса имеют приоритет над `.env`.
Для входа по QR-коду используйте флаг `-qr`:
```powershell
go run ./cmd/telegram-dev -qr
```
Если нужно отключить прокси и работать напрямую (например, через системный VPN), используйте флаг `-no-proxy` или переменную `USE_PROXY=false`.
`TELEGRAM_SESSION_PATH` необязательна: по умолчанию используется
`./var/telegram/<номер без плюса>.session` (или `account-a.session` для `-qr`). Для каждого аккаунта нужен свой файл.
`-limit` принимает значения от 1 до 100 (по умолчанию 20).

## Как вносить изменения

Подробная инструкция по архитектуре, пошаговый чек-лист создания сущностей и распределение задач между участниками команды описаны в [CONTRIBUTING.md](CONTRIBUTING.md).

1. Заберите свежий `main`: `git switch main && git pull`.
2. Создайте ветку под задачу: `git switch -c feature/<что-делаете>`.
3. Коммитьте и пушьте ветку: `git push -u origin feature/<что-делаете>`.
4. Откройте Pull Request в `main` и попросите кого-нибудь из команды посмотреть.

Перед коммитом прогоните `gofmt -w .` и `go vet ./...`.
