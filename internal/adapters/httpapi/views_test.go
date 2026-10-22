package httpapi_test

import (
	"ainfra/internal/adapters/httpapi"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNodeViewsAndPackageTotals(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewStore()
	svc := app.New(store, time.Now, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	node, err := svc.SaveNode(ctx, admin, domain.Node{Name: "large"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	packages := make([]domain.Package, 2000)
	for i := range packages {
		packages[i] = domain.Package{Name: "installed-only", Version: "1", Candidate: "2", Origin: "Ubuntu", SourceVersion: "1"}
	}
	// Historical raw payloads contain bogus zero counters. Reads repair them.
	payload, _ := json.Marshal(map[string]any{"packages": packages, "package_count": 0, "updates": 0, "unknown_origins": 0})
	node.Observations = map[string]domain.Section{"packages": {Data: payload}, "disks": {Data: json.RawMessage(`[{"path":"/srv"}]`)}}
	if err = store.Write(ctx, func(tx app.Tx) error {
		if err := tx.Put("node", node.ID, node); err != nil {
			return err
		}
		return tx.Put("node-summary", node.ID, domain.SummarizeNode(node))
	}); err != nil {
		t.Fatal(err)
	}
	token, _, err := svc.IssueToken(ctx, admin, domain.Principal{Role: "reader"})
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(svc)
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/"+path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"nodes", "nodes/" + node.ID, "nodes?view=summary", "nodes/" + node.ID + "?view=brief"} {
		w := get(path)
		if w.Code != 200 || strings.Contains(w.Body.String(), "installed-only") || w.Body.Len() > 8192 {
			t.Fatalf("%s: %d %d bytes", path, w.Code, w.Body.Len())
		}
	}
	w := get("nodes/" + node.ID + "?view=full")
	var full domain.Node
	if err := json.Unmarshal(w.Body.Bytes(), &full); err != nil {
		t.Fatal(err)
	}
	var data domain.PackageData
	json.Unmarshal(full.Observations["packages"].Data, &data)
	if data.PackageCount == nil || *data.PackageCount != 2000 || data.Updates == nil || *data.Updates != 2000 || len(data.Packages) != 2000 || len(full.Observations["disks"].Data) == 0 {
		t.Fatal("full inventory or totals lost")
	}
	for _, query := range []string{"limit=1&offset=1", "limit=1&upgradable=true"} {
		w := get("nodes/" + node.ID + "/packages?" + query)
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || data.PackageCount == nil || *data.PackageCount != 2000 || len(data.Packages) != 1 || w.Header().Get("X-Total-Count") != "2000" {
			t.Fatalf("bad paginated totals: %s", w.Body.String())
		}
	}
	if w := get("nodes?view=typo"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := get("nodes/" + node.ID + "?view=typo"); w.Code != 400 {
		t.Fatal(w.Code)
	}
}
