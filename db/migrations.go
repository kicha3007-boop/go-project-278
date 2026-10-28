// Package db хранит миграции схемы; приложение применяет их само при старте.
package db

import (
	"embed"
	"io/fs"
)

//go:embed migrations/*.sql
var migrations embed.FS

// MigrationsFS — SQL-миграции goose, корень — каталог db/migrations.
func MigrationsFS() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err) // каталог зашит при сборке, ошибки быть не может
	}

	return sub
}
