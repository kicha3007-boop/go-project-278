#!/bin/sh
# Создаёт (или находит) проект в локальном Bugsink и печатает DSN.
# Нужен запущенный сервис: docker compose --profile monitoring up -d bugsink
# Использование: sh scripts/bugsink-project.sh [имя-проекта]
set -eu

PROJECT_NAME="${1:-go-url-shortener}"
COMPOSE="docker compose --profile monitoring"

TOKEN=$($COMPOSE exec -T bugsink bugsink-manage create_auth_token | tr -d '\r' | tail -n 1)

# Работа с JSON — внутри контейнера Bugsink: там уже есть Python.
$COMPOSE exec -T -e TOKEN="$TOKEN" -e PROJECT_NAME="$PROJECT_NAME" bugsink python - <<'PY'
import json
import os
import urllib.request

API = "http://127.0.0.1:8000/api/canonical/0"
HEADERS = {
    "Authorization": f"Bearer {os.environ['TOKEN']}",
    "Content-Type": "application/json",
}
NAME = os.environ["PROJECT_NAME"]


def call(method, path, payload=None):
    data = json.dumps(payload).encode() if payload is not None else None
    request = urllib.request.Request(
        API + path, data=data, headers=HEADERS, method=method
    )
    with urllib.request.urlopen(request) as response:
        return json.load(response)


def find(path):
    for item in call("GET", path)["results"]:
        if item["name"] == NAME:
            return item
    return None


team = find("/teams/") or call("POST", "/teams/", {"name": NAME})
project = find("/projects/") or call(
    "POST", "/projects/", {"name": NAME, "team": team["id"]}
)
dsn = call("GET", f"/projects/{project['id']}/")["dsn"]

print(f"Проект Bugsink: {NAME}")
print("Для приложения в docker compose (в .env):")
print("SENTRY_DSN=" + dsn.replace("localhost:8000", "bugsink:8000"))
print("Для make run на хосте:")
print("SENTRY_DSN=" + dsn)
PY
