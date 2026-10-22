package app

import (
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"sort"
)

func (s *Service) AppendJournal(ctx context.Context, p domain.Principal, document string, entry domain.JournalEntry) (out domain.JournalEntry, err error) {
	if p.Role != "editor" && p.Role != "admin" {
		return out, ErrForbidden
	}
	if entry.ID == "" || len(entry.ID) > 100 || entry.Step == "" || len(entry.Step) > 300 ||
		!contains([]string{"info", "started", "succeeded", "failed"}, entry.Status) || len(entry.Evidence) > 100 {
		return out, ErrInvalid
	}
	entry.DocumentID, entry.Author, entry.CreatedAt = document, p.ID, s.now()
	raw, e := json.Marshal(entry)
	if e != nil || len(raw) > 1<<20 {
		return out, ErrInvalid
	}
	err = s.store.Write(ctx, func(tx Tx) error {
		var doc domain.Document
		if e := tx.Get("document", document, &doc); e != nil {
			return e
		}
		var existing domain.JournalEntry
		if e := tx.Get("journal-entry", entry.ID, &existing); e == nil {
			// Same id + same content is an idempotent retry, even after another
			// entry was appended. Never silently accept a different payload.
			candidate := entry
			candidate.CreatedAt = existing.CreatedAt
			if hash(candidate) != hash(existing) {
				return ErrConflict
			}
			out = existing
			return nil
		} else if e != ErrNotFound {
			return e
		}
		if e := tx.Put("journal-entry", entry.ID, entry); e != nil {
			return e
		}
		out = entry
		return s.event(tx, p, "document.entry_appended", document, entry.ID)
	})
	return
}

func (s *Service) Journal(ctx context.Context, p domain.Principal, document string) (out []domain.JournalEntry, err error) {
	if !reader(p) {
		return nil, ErrForbidden
	}
	err = s.store.Read(ctx, func(tx Tx) error {
		var doc domain.Document
		if e := tx.Get("document", document, &doc); e != nil {
			return e
		}
		rows, e := tx.ListDocumentEntries(document)
		if e != nil {
			return e
		}
		out = make([]domain.JournalEntry, 0, len(rows))
		for _, raw := range rows {
			var entry domain.JournalEntry
			if e := json.Unmarshal(raw, &entry); e != nil {
				return e
			}
			out = append(out, entry)
		}
		// RFC3339Nano strings are not chronologically ordered when their
		// fractional precision differs. Compare parsed timestamps instead.
		sort.Slice(out, func(i, j int) bool {
			if out[i].CreatedAt.Equal(out[j].CreatedAt) {
				return out[i].ID < out[j].ID
			}
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		})
		return nil
	})
	return
}
