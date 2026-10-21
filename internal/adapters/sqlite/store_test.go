package sqlite_test

import (
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRefuseFutureSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("PRAGMA user_version=99"); e != nil {
		t.Fatal(e)
	}
	db.Close()
	if s, e := sqlite.Open(path); e == nil {
		s.Close()
		t.Fatal("silently opened future schema")
	}
}
func TestUpgradeIndexesExistingDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`CREATE TABLE records(kind TEXT NOT NULL,id TEXT NOT NULL,body TEXT NOT NULL,PRIMARY KEY(kind,id)); INSERT INTO records VALUES('document','old','{"id":"old","title":"Recovery guide","body":"restore backup"}')`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := sqlite.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	rows, e := s.Search(context.Background(), "Recovery", false, 10, 0)
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
}

func TestUpgradeV1MaterializesCompactNodeList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO schema_migrations VALUES(1); CREATE TABLE records(kind TEXT NOT NULL,id TEXT NOT NULL,body TEXT NOT NULL,PRIMARY KEY(kind,id)); PRAGMA user_version=1; INSERT INTO records VALUES('node','n','{"id":"n","name":"old","observations":{"packages":{"status":"ok","complete":true,"observed_at":"2026-10-21T00:00:00Z","data":{"packages":[{"name":"a","version":"1","candidate":"2"}]}}}}')`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := sqlite.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	svc := app.New(s, time.Now, app.RandomID)
	nodes, e := svc.Nodes(context.Background(), domain.Principal{Role: "reader"})
	if e != nil || len(nodes) != 1 {
		t.Fatal(nodes, e)
	}
	var data domain.PackageData
	if e = json.Unmarshal(nodes[0].Observations["packages"].Data, &data); e != nil || data.PackageCount != 1 || data.Updates != 1 || len(data.Packages) != 0 {
		t.Fatal(data, e)
	}
}

func TestSearchResultsAreBoundedAndCounted(t *testing.T) {
	s, e := sqlite.Open(filepath.Join(t.TempDir(), "search.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	if e = s.Write(ctx, func(tx app.Tx) error {
		for _, id := range []string{"a", "b", "c"} {
			if e := tx.Put("document", id, map[string]any{"id": id, "title": "Recovery guide", "body": strings.Repeat("backup ", 10000), "revision": 1}); e != nil {
				return e
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	rows, e := s.Search(ctx, "backup", false, 1, 1)
	if e != nil || len(rows) != 1 || len(rows[0]) > 2048 {
		t.Fatal(len(rows), e)
	}
	count, e := s.CountSearch(ctx, "backup", false)
	if e != nil || count != 3 {
		t.Fatal(count, e)
	}
}
