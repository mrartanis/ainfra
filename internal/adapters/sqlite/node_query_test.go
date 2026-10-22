package sqlite

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestListNodeUsesNodeIndexDespiteOrdering(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	err = s.Write(ctx, func(tx app.Tx) error {
		for _, f := range []domain.Finding{{ID: "z", NodeID: "target"}, {ID: "a", NodeID: "target"}, {ID: "b", NodeID: "other"}} {
			if e := tx.Put("finding", f.ID, f); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.readDB.QueryContext(ctx, "EXPLAIN QUERY PLAN "+listNodeQuery, "finding", "target")
	if err != nil {
		t.Fatal(err)
	}
	indexed := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "USING INDEX records_node (kind=? AND <expr>=?)") {
			indexed = true
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if !indexed {
		t.Fatal("node lookup must not scan all findings for ORDER BY id")
	}
	err = s.Read(ctx, func(tx app.Tx) error {
		raw, e := tx.ListNode("finding", "target")
		if e != nil {
			return e
		}
		if len(raw) != 2 || !strings.Contains(string(raw[0]), `"id":"a"`) || !strings.Contains(string(raw[1]), `"id":"z"`) {
			t.Fatal(raw)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
