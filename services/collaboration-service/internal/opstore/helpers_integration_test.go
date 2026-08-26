//go:build integration

package opstore_test

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func sqlOpen(connStr string) (*sql.DB, error) { return sql.Open("pgx", connStr) }
