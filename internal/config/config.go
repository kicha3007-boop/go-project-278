// Package config читает настройки приложения из переменных окружения.
package config

import (
	"errors"
	"os"
	"strings"
)

// Config — настройки приложения.
type Config struct {
	Host              string   // HOST: адрес прослушивания, пусто — все интерфейсы
	Port              string   // PORT: порт API, по умолчанию 8080
	DatabaseURL       string   // DATABASE_URL: подключение к PostgreSQL, обязательно
	BaseURL           string   // BASE_URL: адрес сайта для short_url
	SentryDSN         string   // SENTRY_DSN: мониторинг ошибок (Sentry/Bugsink), пусто — выключен
	AppEnv            string   // APP_ENV: окружение для мониторинга
	CORSOrigins       []string // CORS_ORIGINS: разрешённые origin через запятую
	TrustedProxies    []string // TRUSTED_PROXIES: адреса прокси, которым верим в X-Forwarded-For
	EnableDebugRoutes bool     // ENABLE_DEBUG_ROUTES: ручка /api/debug/error для проверки мониторинга
}

// Load собирает Config из окружения.
func Load() (Config, error) {
	cfg := Config{
		Host:              os.Getenv("HOST"),
		Port:              getenv("PORT", "8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		SentryDSN:         os.Getenv("SENTRY_DSN"),
		AppEnv:            getenv("APP_ENV", "development"),
		CORSOrigins:       list(getenv("CORS_ORIGINS", "http://localhost:5173")),
		TrustedProxies:    list(getenv("TRUSTED_PROXIES", "127.0.0.1,::1")),
		EnableDebugRoutes: os.Getenv("ENABLE_DEBUG_ROUTES") == "true",
	}
	cfg.BaseURL = strings.TrimRight(getenv("BASE_URL", "http://localhost:"+cfg.Port), "/")

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	return cfg, nil
}

// Addr — адрес для http.Server.
func (c Config) Addr() string {
	return c.Host + ":" + c.Port
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func list(value string) []string {
	var items []string

	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}

	return items
}
