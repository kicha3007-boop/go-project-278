#!/usr/bin/env bash
# Запуск в контейнере: миграции, затем Caddy (вход, :PORT) и приложение (127.0.0.1:BACKEND_PORT).
# Если любой из процессов завершился, гасим второй и выходим с ошибкой,
# чтобы Docker/Render перезапустили контейнер целиком.
set -euo pipefail

PORT="${PORT:-80}"
BACKEND_PORT="${BACKEND_PORT:-8080}"

if [ "$PORT" = "$BACKEND_PORT" ]; then
    echo "[run.sh] PORT ($PORT) совпадает с BACKEND_PORT: Caddy и приложение не уживутся" >&2
    exit 1
fi

echo "[run.sh] Starting service"

echo "[run.sh] Running DB migrations"
goose -dir ./db/migrations postgres "${DATABASE_URL}" up

echo "[run.sh] Starting Caddy"
caddy run --config /etc/caddy/Caddyfile --adapter caddyfile &

echo "[run.sh] Starting Go app"
HOST=127.0.0.1 PORT="$BACKEND_PORT" /app/bin/app &

stop_all() {
    kill -TERM $(jobs -p) 2>/dev/null || true
}
trap stop_all TERM INT

set +e
wait -n
status=$?
stop_all
wait
exit "$status"
