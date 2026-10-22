package contracts_test

import (
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendOnlyJournalContract(t *testing.T) {
	for _, kind := range []string{"fake", "sqlite"} {
		t.Run(kind, func(t *testing.T) {
			var store app.Store = testkit.NewStore()
			if kind == "sqlite" {
				s, err := sqlite.Open(filepath.Join(t.TempDir(), "state.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				store = s
			}
			ctx := context.Background()
			now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
			svc := app.New(store, func() time.Time { return now }, app.RandomID)
			editor := domain.Principal{ID: "operator", Role: "editor"}
			admin := domain.Principal{ID: "owner", Role: "admin"}
			doc, err := svc.EditDocument(ctx, editor, domain.Document{Title: "Maintenance", Kind: "maintenance_plan", Body: "approved plan"}, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = svc.ApprovePlan(ctx, admin, doc.ID, doc.Revision); err != nil {
				t.Fatal(err)
			}
			entry := domain.JournalEntry{ID: "stable-retry-id", Step: "Upgrade", Status: "succeeded", Command: "apt-get upgrade", Result: "exit 0", Evidence: []string{}, Rollback: "snapshot"}
			original, err := svc.AppendJournal(ctx, editor, doc.ID, entry)
			if err != nil {
				t.Fatal(err)
			}
			now = now.Add(time.Millisecond)
			retry, err := svc.AppendJournal(ctx, editor, doc.ID, entry)
			if err != nil || retry.ID != original.ID || !retry.CreatedAt.Equal(original.CreatedAt) {
				t.Fatal(retry, err)
			}
			entry.Result = "different"
			if _, err := svc.AppendJournal(ctx, editor, doc.ID, entry); !errors.Is(err, app.ErrConflict) {
				t.Fatal("changed retry accepted", err)
			}
			if _, err := svc.AppendJournal(ctx, domain.Principal{Role: "reader"}, doc.ID, entry); !errors.Is(err, app.ErrForbidden) {
				t.Fatal(err)
			}
			entry.ID = "second"
			if _, err := svc.AppendJournal(ctx, editor, doc.ID, entry); err != nil {
				t.Fatal(err)
			}
			rows, err := svc.Journal(ctx, editor, doc.ID)
			if err != nil || len(rows) != 2 || rows[0].ID != original.ID {
				t.Fatal(rows, err)
			}
			unchanged, err := svc.Document(ctx, editor, doc.ID, 0)
			if err != nil || unchanged.Revision != doc.Revision || unchanged.Status != "approved" || unchanged.Body != doc.Body {
				t.Fatal("append modified approved plan", unchanged, err)
			}
			// Entries are isolated from another document.
			other, err := svc.EditDocument(ctx, editor, domain.Document{Title: "Other", Kind: "note"}, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.AppendJournal(ctx, editor, other.ID, entry); !errors.Is(err, app.ErrConflict) {
				t.Fatal("ID reused across documents", err)
			}
			rows, err = svc.Journal(ctx, editor, other.ID)
			if err != nil || len(rows) != 0 {
				t.Fatal(rows, err)
			}
		})
	}
}
