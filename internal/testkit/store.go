package testkit

import (
	"ainfra/internal/app"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Store struct {
	mu   sync.Mutex
	data map[string]json.RawMessage
}
type tx struct{ data map[string]json.RawMessage }

func NewStore() *Store { return &Store{data: map[string]json.RawMessage{}} }
func IDs() func() string {
	n := 0
	return func() string { n++; return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }
}
func (t tx) Get(k, id string, out any) error {
	b, ok := t.data[k+"/"+id]
	if !ok {
		return app.ErrNotFound
	}
	return json.Unmarshal(b, out)
}
func (t tx) Put(k, id string, v any) error {
	b, e := json.Marshal(v)
	if e == nil {
		t.data[k+"/"+id] = b
	}
	return e
}
func (t tx) Delete(k, id string) error { delete(t.data, k+"/"+id); return nil }
func (t tx) List(k string) ([]json.RawMessage, error) {
	keys := []string{}
	for key := range t.data {
		if strings.HasPrefix(key, k+"/") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := []json.RawMessage{}
	for _, key := range keys {
		out = append(out, t.data[key])
	}
	return out, nil
}
func (s *Store) Read(ctx context.Context, f func(app.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return f(tx{s.data})
}
func (s *Store) Write(ctx context.Context, f func(app.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := map[string]json.RawMessage{}
	for k, v := range s.data {
		cp[k] = append([]byte(nil), v...)
	}
	if err := f(tx{cp}); err != nil {
		return err
	}
	s.data = cp
	return nil
}
func (s *Store) Search(ctx context.Context, q string, literal bool, limit, offset int) (out []json.RawMessage, err error) {
	out = []json.RawMessage{}
	err = s.Read(ctx, func(t app.Tx) error {
		for _, k := range []string{"node", "document"} {
			rows, _ := t.List(k)
			for _, r := range rows {
				if strings.Contains(strings.ToLower(string(r)), strings.ToLower(q)) {
					out = append(out, r)
				}
			}
		}
		return nil
	})
	if offset >= len(out) {
		return []json.RawMessage{}, err
	}
	out = out[offset:]
	if len(out) > limit {
		out = out[:limit]
	}
	return
}
