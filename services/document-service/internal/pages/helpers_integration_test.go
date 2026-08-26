//go:build integration

package pages_test

import (
	"database/sql"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func sqlOpen(connStr string) (*sql.DB, error) { return sql.Open("pgx", connStr) }

func testUUID() uuid.UUID { return uuid.Must(uuid.NewV7()) }
