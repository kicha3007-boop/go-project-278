# Переменные из .env (если есть) доступны целям ниже.
-include .env
export

PORT ?= 8080
TEST_DATABASE_URL ?= postgres://shortener:shortener@localhost:5432/shortener_test?sslmode=disable
FRONTEND_PKG = node_modules/@hexlet/project-url-shortener-frontend

.PHONY: install build start dev backend frontend test lint lint-fix sqlc migrate \
	db compose-up compose-down monitoring bugsink-project

install:
	go mod download
	npm ci

build:
	go build -o bin/app .

# Приложение на :8080, миграции применяются при старте. Нужен DATABASE_URL (.env.example, make db).
start: build
	./bin/app

backend:
	go run .

# UI на http://localhost:5173, /api проксируется на API_URL (по умолчанию :8080).
# На Windows штатная команда пакета молча завершается (зовёт npm без shell),
# поэтому там запускаем тот же `vite preview` напрямую.
frontend:
ifeq ($(OS),Windows_NT)
	npm --prefix $(FRONTEND_PKG) run preview
else
	npx start-hexlet-url-shortener-frontend
endif

# Бэкенд и UI вместе.
dev:
	npx concurrently --kill-others --names backend,frontend "$(MAKE) backend" "$(MAKE) frontend"

# Тесты на отдельной базе shortener_test (создаётся в make db).
test:
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -count=1 ./...

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...

# Перегенерировать internal/store из db/queries (нужен Docker).
sqlc:
	docker run --rm -v "$(CURDIR):/src" -w /src sqlc/sqlc:1.31.1 generate

migrate:
	go tool goose -dir ./db/migrations postgres "$(DATABASE_URL)" up

# Только Postgres из compose — для make start / make dev / make test на хосте.
db:
	docker compose up -d db

compose-up:
	docker compose up --build

compose-down:
	docker compose --profile monitoring down -v

# Приложение + локальный Bugsink (http://localhost:8000).
monitoring:
	docker compose --profile monitoring up --build -d

# Создать проект в локальном Bugsink и напечатать DSN для .env.
bugsink-project:
	sh scripts/bugsink-project.sh
