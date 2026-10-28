# Сокращатель ссылок (Go)

[![hexlet-check](https://github.com/kicha3007-boop/go-project-278/actions/workflows/hexlet-check.yml/badge.svg)](https://github.com/kicha3007-boop/go-project-278/actions)
[![CI](https://github.com/kicha3007-boop/go-project-278/actions/workflows/ci.yml/badge.svg)](https://github.com/kicha3007-boop/go-project-278/actions/workflows/ci.yml)

Сервис коротких ссылок: API на Go (Gin, PostgreSQL, sqlc, goose), переход по короткой ссылке с учётом
посещений, веб-интерфейс из пакета Хекслета за Caddy, мониторинг ошибок через Sentry SDK (Bugsink).

Развёрнутой копии нет: проект запускается локально одной командой — так же, как на Render
(тот же Dockerfile, Caddy и миграции при старте).

## Быстрый старт (Docker)

```bash
git clone https://github.com/kicha3007-boop/go-project-278.git
cd go-project-278
docker compose up --build        # UI и API: http://localhost:8090
```

С мониторингом ошибок (локальный Bugsink, http://localhost:8000, вход `admin@example.com` / `admin`):

```bash
docker compose --profile monitoring up --build -d
make bugsink-project             # создаёт проект и печатает SENTRY_DSN
# SENTRY_DSN=… и ENABLE_DEBUG_ROUTES=true в .env, затем:
docker compose up -d app
curl http://localhost:8090/api/debug/error   # событие появится в Bugsink
```

## Разработка на хосте

Нужны Go 1.25+, Node.js 20+, Docker (для Postgres), golangci-lint.

```bash
cp .env.example .env
make install     # зависимости Go и UI
make db          # Postgres из compose (базы shortener и shortener_test)
make start       # сборка и запуск API на :8080, миграции применяются при старте
make dev         # API и UI вместе: UI на http://localhost:5173
make test        # тесты API на базе shortener_test
make lint        # golangci-lint
```

## Переменные окружения

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `DATABASE_URL` | — (обязательна) | подключение к PostgreSQL |
| `BASE_URL` | `http://localhost:$PORT` | адрес сайта для поля `short_url` |
| `HOST`, `PORT` | все интерфейсы, `8080` | адрес прослушивания API |
| `SENTRY_DSN` | пусто — выключен | DSN Sentry/Bugsink |
| `CORS_ORIGINS` | `http://localhost:5173` | разрешённые origin через запятую |
| `TRUSTED_PROXIES` | `127.0.0.1,::1` | прокси, которым верим в `X-Forwarded-For` |
| `ENABLE_DEBUG_ROUTES` | `false` | ручка `/api/debug/error` для проверки мониторинга |
| `TEST_DATABASE_URL` | база `shortener_test` | отдельная база для тестов |

В контейнере Caddy слушает `PORT` (80), приложение — `127.0.0.1:BACKEND_PORT` (8080), см. `bin/run.sh`.

## API

| Метод и путь | Ответ |
|---|---|
| `GET /ping` | `200 pong` |
| `GET /api/links?range=[0,9]` | `200`, список ссылок, заголовок `Content-Range: links 0-9/42` |
| `POST /api/links` | `201`, созданная ссылка; `short_name` необязателен — сгенерируется |
| `GET /api/links/:id` | `200` или `404` |
| `PUT /api/links/:id` | `200` или `404` |
| `DELETE /api/links/:id` | `204` или `404` |
| `GET /r/:code` | `302` на исходный адрес, посещение записывается |
| `GET /api/link_visits` (`?range=` или заголовок `Range: [0,9]`) | `200`, список посещений, `Content-Range: link_visits …` |

Диапазон — как у `ra-data-simple-rest`: границы включительно. Ошибки: `404`/`400` — `{"error": "…"}`,
валидация — `422 {"errors": {"<поле>": "<сообщение>"}}` (`original_url` — обязательный URL,
`short_name` — 3–32 символа `[A-Za-z0-9_-]`, уникален), невалидный JSON — `400 {"error": "invalid request"}`.

```bash
curl -X POST http://localhost:8090/api/links \
  -H "Content-Type: application/json" \
  -d '{"original_url": "https://example.com/long-url", "short_name": "exmpl"}'
# {"id":1,"original_url":"https://example.com/long-url","short_name":"exmpl","short_url":"http://localhost:8090/r/exmpl","created_at":"…"}

curl -i http://localhost:8090/r/exmpl          # 302, Location: https://example.com/long-url
curl -i "http://localhost:8090/api/link_visits?range=[0,9]"
```

## Устройство

- `main.go` — собирает настройки, применяет миграции, поднимает сервер;
- `internal/server` — маршруты (`router.go`), обработчики ссылок и посещений, пагинация, валидация;
- `internal/links` — операции над ссылками поверх `internal/store` (код sqlc из `db/queries`);
- `db/migrations` — миграции goose (встроены в бинарник, `go tool goose` для ручного запуска);
- `Dockerfile`, `Caddyfile`, `bin/run.sh` — образ для Render и локального запуска.

Проверка Хекслета локально: образ `hexletprojects/go_url_shortener_project_ru` содержит тесты
автопроверки (`tests/`) и свой `docker-compose.yml`; код кладётся в `code/`, затем
`docker compose -f docker-compose.yml run --rm app make setup && docker compose -f docker-compose.yml run --rm test`.
