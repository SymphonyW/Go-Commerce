package dbmigrations

import (
	"embed"

	"github.com/pressly/goose/v3"
)

const Dir = "migrations"

// FS embeds the SQL migration files so the migrate command can run from a
// compiled container without mounting the source tree.
//
//go:embed migrations/*.sql
var FS embed.FS

func Configure() error {
	goose.SetBaseFS(FS)
	return goose.SetDialect("mysql")
}
