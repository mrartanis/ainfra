package sqlite_test

import (
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/app"
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestPersistenceRollbackFTSBackup(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	s, e := sqlite.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	e = s.Write(ctx, func(tx app.Tx) error {
		return tx.Put("document", "one", map[string]any{"id": "one", "title": "Backup guide", "body": "restic 10.2.3.4"})
	})
	if e != nil {
		t.Fatal(e)
	}
	e = s.Write(ctx, func(tx app.Tx) error {
		if e := tx.Delete("document", "one"); e != nil {
			return e
		}
		return app.ErrConflict
	})
	if !errors.Is(e, app.ErrConflict) {
		t.Fatal(e)
	}
	for _, literal := range []bool{true, false} {
		q := "Backup"
		if literal {
			q = "10.2.3.4"
		}
		v, e := s.Search(ctx, q, literal, 10, 0)
		if e != nil || len(v) != 1 {
			t.Fatalf("search %v: %s %v", literal, v, e)
		}
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	if e = s.Backup(ctx, backup); e != nil {
		t.Fatal(e)
	}
	b, e := sqlite.Open(backup)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	if e = b.Read(ctx, func(tx app.Tx) error { var v map[string]any; return tx.Get("document", "one", &v) }); e != nil {
		t.Fatal(e)
	}
}
