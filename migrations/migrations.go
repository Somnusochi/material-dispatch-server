package migrations

import (
	"database/sql"
	"embed"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed *.sql
var files embed.FS

func New(db *sql.DB) (*goose.Provider, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, db, files, goose.WithSessionLocker(locker), goose.WithTableName("public.goose_db_version"))
}
