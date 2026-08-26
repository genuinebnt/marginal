// Package migrate applies collaboration-service's Postgres schema. One
// function, used by both cmd/main.go at startup and integration tests
// against a fresh testcontainers Postgres — the schema a real deployment
// runs is exactly the schema tests run against.
package migrate

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Up applies every pending migration in migrations/. db must be a
// database/sql handle (e.g. via pgx's stdlib adapter) — goose's migration
// runner is database/sql-based, independent of the pgxpool.Pool the rest
// of the service uses for its own queries.
func Up(db *sql.DB) error {
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrate: set dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}
	return nil
}
