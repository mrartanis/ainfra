package sqlite_test

import (
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"path/filepath"
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
