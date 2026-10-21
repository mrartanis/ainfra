package contracts_test

import (
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// The same observable contract runs against both implementations of the port.
func TestStorageContract(t *testing.T) {
	factories := map[string]func(*testing.T) app.Store{
		"fake": func(*testing.T) app.Store { return testkit.NewStore() },
		"sqlite": func(t *testing.T) app.Store {
			s, e := sqlite.Open(filepath.Join(t.TempDir(), "contract.db"))
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { s.Close() })
			return s
		},
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := factory(t)
			put := func(tx app.Tx, kind, id, value string) error {
				return tx.Put(kind, id, map[string]string{"id": id, "value": value, "node_id": "n"})
			}
			if e := s.Write(ctx, func(tx app.Tx) error {
				if e := put(tx, "records", "b", "second"); e != nil {
					return e
				}
				return put(tx, "records", "a", "first")
			}); e != nil {
				t.Fatal(e)
			}
			rollback := errors.New("abort")
			if e := s.Write(ctx, func(tx app.Tx) error {
				if e := tx.Delete("records", "a"); e != nil {
					return e
				}
				if e := put(tx, "records", "b", "changed"); e != nil {
					return e
				}
				return rollback
			}); !errors.Is(e, rollback) {
				t.Fatal(e)
			}
			if e := s.Read(ctx, func(tx app.Tx) error {
				rows, e := tx.List("records")
				if e != nil {
					return e
				}
				if len(rows) != 2 {
					t.Fatal(rows)
				}
				var first map[string]string
				json.Unmarshal(rows[0], &first)
				if first["id"] != "a" || first["value"] != "first" {
					t.Fatal(first)
				}
				// Returned buffers belong to the caller, never to committed store state.
				rows[0][0] = '!'
				var value map[string]string
				if e = tx.Get("records", "a", &value); e != nil {
					return e
				}
				if e = tx.Get("records", "b", &value); e != nil {
					return e
				}
				if value["value"] != "second" {
					t.Fatal(value)
				}
				scoped, e := tx.ListNode("records", "n")
				if e != nil {
					return e
				}
				if len(scoped) != 2 {
					t.Fatal(scoped)
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			t.Run("read_only", func(t *testing.T) {
				if e := s.Read(ctx, func(tx app.Tx) error { return put(tx, "records", "readonly", "bad") }); e == nil {
					t.Fatal("Read allowed mutation")
				}
			})
			t.Run("tuple_identity", func(t *testing.T) {
				if e := s.Write(ctx, func(tx app.Tx) error {
					if e := put(tx, "x/y", "z", "one"); e != nil {
						return e
					}
					return put(tx, "x", "y/z", "two")
				}); e != nil {
					t.Fatal(e)
				}
				if e := s.Read(ctx, func(tx app.Tx) error {
					var row map[string]string
					if e := tx.Get("x/y", "z", &row); e != nil {
						return e
					}
					if row["value"] != "one" {
						t.Fatal("kind/id collision", row)
					}
					rows, e := tx.List("x")
					if e != nil {
						return e
					}
					if len(rows) != 1 {
						t.Fatal("kind prefix leaked", rows)
					}
					return nil
				}); e != nil {
					t.Fatal(e)
				}
			})
			t.Run("revision_and_literal_search", func(t *testing.T) {
				service := app.New(s, time.Now, testkit.IDs())
				admin := domain.Principal{Role: "admin"}
				original, e := service.EditDocument(ctx, admin, domain.Document{Title: "Contract", Kind: "note", Body: "needle.contract.192.0.2.1", Tags: []string{"backup"}}, 0)
				if e != nil {
					t.Fatal(e)
				}
				changed := original
				changed.Body += " updated"
				current, e := service.EditDocument(ctx, admin, changed, original.Revision)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = service.EditDocument(ctx, admin, original, original.Revision); !errors.Is(e, app.ErrConflict) {
					t.Fatal("stale revision accepted", e)
				}
				historical, e := service.Document(ctx, admin, original.ID, original.Revision)
				if e != nil || historical.Body != original.Body || historical.Tags[0] != "backup" {
					t.Fatal(historical, e)
				}
				rows, e := s.Search(ctx, "needle.contract.192.0.2.1", true, 1, 0)
				if e != nil || len(rows) != 1 {
					t.Fatal(rows, e)
				}
				var hit struct {
					ID       string `json:"id"`
					Revision int    `json:"revision"`
				}
				if e = json.Unmarshal(rows[0], &hit); e != nil {
					t.Fatal(e)
				}
				count, e := s.CountSearch(ctx, "needle.contract.192.0.2.1", true)
				if e != nil || count != 1 || hit.ID != current.ID || hit.Revision != current.Revision {
					t.Fatal(hit, count, e)
				}
			})
			t.Run("cancellation", func(t *testing.T) {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				if e := s.Write(cancelled, func(tx app.Tx) error { return put(tx, "records", "cancelled", "bad") }); e == nil {
					t.Fatal("cancelled transaction committed")
				}
			})
		})
	}
}
