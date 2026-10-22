package httpapi_test

import (
	"ainfra/internal/adapters/httpapi"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestListPaginationBoundsAndStableOrder(t *testing.T) {
	s := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	ctx := context.Background()
	admin := domain.Principal{Role: "admin"}
	for i := 0; i < 105; i++ {
		if _, e := s.SaveNode(ctx, admin, domain.Node{Name: fmt.Sprintf("node-%03d", i)}, 0); e != nil {
			t.Fatal(e)
		}
	}
	token, _, e := s.IssueToken(ctx, admin, domain.Principal{Role: "reader"})
	if e != nil {
		t.Fatal(e)
	}
	h := httpapi.New(s)
	for _, tc := range []struct {
		query         string
		status, count int
	}{{"", 200, 100}, {"?limit=20&offset=100", 200, 5}, {"?limit=1001", 400, 0}, {"?offset=-1", 400, 0}, {"?limit=banana", 400, 0}} {
		r := httptest.NewRequest("GET", "/api/v1/nodes"+tc.query, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s status %d", tc.query, w.Code)
		}
		if w.Code == 200 {
			var rows []domain.Node
			if e = json.Unmarshal(w.Body.Bytes(), &rows); e != nil || len(rows) != tc.count {
				t.Fatalf("%s count %d %v", tc.query, len(rows), e)
			}
			if w.Header().Get("X-Total-Count") != "105" {
				t.Fatal(w.Header())
			}
		}
	}
}

func (brokenStore) CountSearch(context.Context, string, bool) (int, error) {
	return 0, errors.New("database unavailable")
}

func TestAuthenticationStorageFailureIsNotInvalidCredentials(t *testing.T) {
	h := httpapi.New(app.New(brokenStore{}, time.Now, testkit.IDs()))
	r := httptest.NewRequest("GET", "/api/v1/me", nil)
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 64))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 500 || strings.Contains(w.Body.String(), "database unavailable") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestRelationSuggestionsReadableAndPaginated(t *testing.T) {
	s := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	token, _, err := s.IssueToken(context.Background(), domain.Principal{Role: "admin"}, domain.Principal{Role: "reader"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/v1/relation-suggestions?limit=1", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	httpapi.New(s).ServeHTTP(w, r)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" || w.Header().Get("X-Total-Count") != "0" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
}

func TestAttentionHostingFilterBeforePagination(t *testing.T) {
	s := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	ctx := context.Background()
	admin := domain.Principal{Role: "admin"}
	host, _ := s.SaveNode(ctx, admin, domain.Node{Name: "host", Labels: map[string]string{"provider": "ExampleProvider"}}, 0)
	for i := 0; i < 4; i++ {
		n, _ := s.SaveNode(ctx, admin, domain.Node{Name: fmt.Sprintf("vm%d", i)}, 0)
		if _, e := s.SaveRelation(ctx, admin, domain.Relation{Source: n.ID, Target: host.ID, Type: "runs_on"}); e != nil {
			t.Fatal(e)
		}
	}
	s.SaveNode(ctx, admin, domain.Node{Name: "elsewhere"}, 0)
	token, _, e := s.IssueToken(ctx, admin, domain.Principal{Role: "reader"})
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("GET", "/api/v1/attention?provider=ExampleProvider&host="+host.ID+"&sort=name&limit=2&offset=2", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	httpapi.New(s).ServeHTTP(w, r)
	var rows []domain.Attention
	e = json.Unmarshal(w.Body.Bytes(), &rows)
	if w.Code != 200 || e != nil || len(rows) != 2 || rows[0].Name != "vm1" || w.Header().Get("X-Total-Count") != "5" || w.Header().Get("X-Next-Offset") != "4" {
		t.Fatal(w.Code, w.Header(), w.Body.String(), e)
	}
}
