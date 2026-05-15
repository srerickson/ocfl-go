package stagedb

import (
	"context"
	"database/sql"
	"embed"
	"log"

	_ "embed"

	"github.com/srerickson/ocfl-go/internal/stagedb/generated"
	_ "modernc.org/sqlite"
)

const sqlitePragmas = "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on"

type File struct {
	db *sql.DB
}

//go:embed schema/*.sql
var schemaFS embed.FS

func Open(path string) (*File, error) {
	db, err := sql.Open("sqlite", path+sqlitePragmas)
	if err != nil {
		return nil, err
	}

	f := &File{db: db}
	if err := f.migrate(context.Background()); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *File) Close() error {
	return f.db.Close()
}

func (f File) migrate(ctx context.Context) error {

	var schemaVersion int
	gen := generated.New(f.db)

	schemaVersionsExists := `SELECT count(*) 
		FROM sqlite_master 
		WHERE type = 'table' AND tbl_name = "schema_version";`

	tableNames, err := f.db.QueryContext(ctx, schemaVersionsExists)
	if err != nil {
		return err
	}

	if tableNames.Next() {
		var count int64
		if err := tableNames.Scan(&count); err != nil {
			return err
		}
		tableNames.Close()
		if count > 0 {
			v, err := gen.GetSchemaVersion(ctx)
			if err != nil {
				return err
			}
			schemaVersion = int(v)
		}
	}

	log.Println("schema version", schemaVersion)

	// migrations, err := fs.ReadDir(schemaFS, ".")
	// if err != nil {
	// 	return nil
	// }

	// gen := generated.New(f.db)

	// f.db.Conn()
	// conn := f.db.
	// 	conn.QueryContext(ctx, "")
	// for _, migration := range migrations {
	// 	migrationBytes, err := fs.ReadFile(schemaFS, migration.Name())
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }
	return nil
}
