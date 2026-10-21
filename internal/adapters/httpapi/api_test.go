package httpapi_test

import (
	"ainfra/internal/adapters/httpapi"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIWriteOnlyAndBodyLimits(t *testing.T) {
	s := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	ctx := context.Background()
	admin := domain.Principal{Role: "admin"}
	n, _ := s.SaveNode(ctx, admin, domain.Node{Name: "host"}, 0)
	token, _, e := s.IssueToken(ctx, admin, domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "i"})
	if e != nil {
		t.Fatal(e)
	}
	h := httpapi.New(s)
	for _, path := range []string{"/api/v1/nodes", "/api/v1/search?q=x", "/api/v1/export", "/api/v1/documents", "/api/v1/feed-status"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("POST", "/api/v1/ingest/reports", strings.NewReader(strings.Repeat("x", httpapi.MaxBody+1)))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatalf("size %d", w.Code)
	}
	if strings.Contains(w.Body.String(), token) {
		t.Fatal("secret leaked")
	}
}

func TestReadinessChecksStorage(t *testing.T) {
	s := app.New(brokenStore{}, time.Now, testkit.IDs())
	h := httpapi.New(s)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 {
		t.Fatalf("readiness %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatalf("health %d", w.Code)
	}
}

type brokenStore struct{}

func (brokenStore) Read(context.Context, func(app.Tx) error) error {
	return errors.New("database unavailable")
}
func (brokenStore) Write(context.Context, func(app.Tx) error) error {
	return errors.New("database unavailable")
}
func (brokenStore) Search(context.Context, string, bool, int, int) ([]json.RawMessage, error) {
	return nil, errors.New("database unavailable")
}
