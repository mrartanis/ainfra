package sqlite_test

import (
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRetentionBoundsHistoryPreservesCurrentAndOpen(t *testing.T) {
	s, e := sqlite.Open(filepath.Join(t.TempDir(), "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	e = s.Write(ctx, func(tx app.Tx) error {
		for _, kind := range []string{"receipt", "event", "revision"} {
			for i := 1; i <= 5; i++ {
				id := string(rune('a' + i))
				v := map[string]any{"id": "doc", "revision": i, "at": now.AddDate(0, 0, -100), "received_at": now.AddDate(0, 0, -100)}
				if e := tx.Put(kind, id, v); e != nil {
					return e
				}
			}
		}
		if e := tx.Put("document", "doc", domain.Document{ID: "doc", Revision: 5, ApprovedRevision: 1}); e != nil {
			return e
		}
		if e := tx.Put("finding", "open", domain.Finding{ID: "open", Workflow: "open", LastEvaluated: now.AddDate(0, 0, -100)}); e != nil {
			return e
		}
		return tx.Put("finding", "resolved", domain.Finding{ID: "resolved", Workflow: "resolved", LastEvaluated: now.AddDate(0, 0, -100)})
	})
	if e != nil {
		t.Fatal(e)
	}
	result, e := s.Prune(ctx, sqlite.Retention{Days: 30, MaxHistory: 2, DocumentRevisions: 2}, now)
	if e != nil {
		t.Fatal(e)
	}
	if result.Deleted < 10 {
		t.Fatal(result)
	}
	e = s.Read(ctx, func(tx app.Tx) error {
		for _, kind := range []string{"receipt", "event"} {
			rows, e := tx.List(kind)
			if e != nil || len(rows) > 2 {
				t.Fatalf("%s %d %v", kind, len(rows), e)
			}
		}
		rows, _ := tx.List("revision")
		if len(rows) != 3 {
			t.Fatalf("expected latest 2 + approved revision: %d", len(rows))
		}
		var f domain.Finding
		return tx.Get("finding", "open", &f)
	})
	if e != nil {
		t.Fatal(e)
	}
}

func TestResolvedRetentionUsesClosureTimeAndHistoryCap(t *testing.T) {
	s, err := sqlite.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	err = s.Write(ctx, func(tx app.Tx) error {
		for i, id := range []string{"a", "b", "c"} {
			if err := tx.Put("event", id, map[string]any{"at": now.Add(time.Duration(i) * time.Second)}); err != nil {
				return err
			}
		}
		return tx.Put("finding", "closed", map[string]any{"workflow": "resolved", "resolved_at": now.AddDate(0, 0, -100), "last_evaluated": now})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Prune(ctx, sqlite.Retention{Days: 30, MaxHistory: 2, DocumentRevisions: 2}, now); err != nil {
		t.Fatal(err)
	}
	err = s.Read(ctx, func(tx app.Tx) error {
		rows, err := tx.List("finding")
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatal("reassessment extended closed finding retention")
		}
		rows, err = tx.List("event")
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("history cap: %d", len(rows))
		}
		var v map[string]any
		if err := tx.Get("event", "a", &v); err != app.ErrNotFound {
			t.Fatalf("oldest recent event retained: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseSizeLimitRollsBackOversizedWrite(t *testing.T) {
	s, err := sqlite.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.SetSizeLimit(ctx, 64); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx app.Tx) error { return tx.Put("sentinel", "keep", map[string]string{"value": "original"}) }); err != nil {
		t.Fatal(err)
	}
	oversized := strings.Repeat("x", 65<<20)
	err = s.Write(ctx, func(tx app.Tx) error {
		if err := tx.Put("sentinel", "keep", map[string]string{"value": "changed"}); err != nil {
			return err
		}
		return tx.Put("blob", "oversized", map[string]string{"value": oversized})
	})
	if err == nil {
		t.Fatal("write exceeded configured database size")
	}
	err = s.Read(ctx, func(tx app.Tx) error {
		var v map[string]string
		if err := tx.Get("sentinel", "keep", &v); err != nil {
			return err
		}
		if v["value"] != "original" {
			t.Fatal("failed transaction was partially committed")
		}
		if err := tx.Get("blob", "oversized", &v); err != app.ErrNotFound {
			t.Fatalf("oversized record persisted: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
