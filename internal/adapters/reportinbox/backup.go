package reportinbox

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
)

// Backup copies a running inbox consistently without claiming the worker lock or
// resetting processing state. Restore accepts processing rows for idempotent replay.
func Backup(ctx context.Context, source, output string) error {
	if _, err := os.Stat(source); err != nil {
		return err
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		return os.ErrExist
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	f.Close()
	db, err := sql.Open("sqlite", source)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, "VACUUM INTO ?", output)
	return err
}
