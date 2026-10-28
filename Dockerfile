# syntax=docker/dockerfile:1
# Один образ для Render и для локального запуска: Caddy (вход, :PORT) раздаёт UI
# и проксирует API на приложение (127.0.0.1:8080), перед стартом применяются миграции.

# 1) UI: готовая сборка из npm-пакета Хекслета
FROM node:24-alpine AS frontend-builder
WORKDIR /build/frontend
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --ignore-scripts --no-audit --no-fund

# 2) Приложение и goose
FROM golang:1.25-alpine AS backend-builder
RUN apk add --no-cache git
WORKDIR /build/code

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Версия goose берётся из go.mod (директива tool), а не из @latest.
RUN --mount=type=cache,target=/go/pkg/mod \
    go build -o /build/goose github.com/pressly/goose/v3/cmd/goose

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /build/app .

# 3) Runtime
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata bash caddy

WORKDIR /app
ENV PORT=80 BACKEND_PORT=8080

COPY --from=backend-builder /build/app /app/bin/app
COPY --from=backend-builder /build/goose /usr/local/bin/goose
COPY --from=backend-builder /build/code/db/migrations /app/db/migrations
COPY --from=frontend-builder \
    /build/frontend/node_modules/@hexlet/project-url-shortener-frontend/dist \
    /app/public
COPY Caddyfile /etc/caddy/Caddyfile
COPY bin/run.sh /app/bin/run.sh
RUN chmod +x /app/bin/run.sh

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:${PORT}/ping" >/dev/null || exit 1

CMD ["/app/bin/run.sh"]
