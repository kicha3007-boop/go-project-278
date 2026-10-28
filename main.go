// Сокращатель ссылок: API на Gin и PostgreSQL. Точка входа собирает настройки и поднимает сервер.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"

	"url-shortener/db"
	"url-shortener/internal/config"
	"url-shortener/internal/links"
	"url-shortener/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// .env нужен только локально; на хостинге переменные задаются окружением, файла нет.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if cfg.SentryDSN != "" {
		if err := sentry.Init(sentry.ClientOptions{Dsn: cfg.SentryDSN, Environment: cfg.AppEnv}); err != nil {
			return fmt.Errorf("sentry: %w", err)
		}
		defer sentry.Flush(2 * time.Second)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	if err := migrate(ctx, pool); err != nil {
		return err
	}

	router, err := server.NewRouter(cfg, links.NewService(pool))
	if err != nil {
		return fmt.Errorf("router: %w", err)
	}

	srv := &http.Server{Addr: cfg.Addr(), Handler: router, ReadHeaderTimeout: 10 * time.Second}

	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	log.Printf("listening on %s", cfg.Addr())

	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	return nil
}

// migrate применяет миграции из db/migrations — приложение стартует одной командой.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	// Закрытие *sql.DB не закрывает сам пул, им дальше пользуется приложение.
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Printf("close migrations connection: %v", err)
		}
	}()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, db.MigrationsFS())
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	return nil
}
