package ui

import (
	"ainfra/internal/apiclient"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestEditAndDeleteEmbeddedRecordsPreserveObservations(t *testing.T) {
	for _, tc := range []struct {
		path, field string
		remove      bool
	}{{"/services/save", "services", false}, {"/services/delete", "services", true}, {"/backups/save", "backups", false}, {"/backups/delete", "backups", true}} {
		t.Run(tc.path, func(t *testing.T) {
			original := map[string]any{"name": "old", "id": "backup", "primary": true, "last_success_at": "2026-10-21T12:00:00Z", "observed_at": "2026-10-21T13:00:00Z"}
			var saved map[string]any
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					json.NewEncoder(w).Encode(map[string]any{"id": "node", "revision": 7, tc.field: []any{original, map[string]any{"name": "untouched", "id": "other"}}})
					return
				}
				if r.Method != "PATCH" || r.URL.Path != "/api/v1/nodes/node" || r.Header.Get("If-Match") != "7" {
					t.Error(r.Method, r.URL.Path, r.Header.Get("If-Match"))
				}
				if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
					t.Error(err)
				}
				w.Write([]byte(`{}`))
			}))
			defer api.Close()
			u := New(api.URL, api.Client(), false, time.Now)
			c := &apiclient.Client{Base: api.URL, HTTP: api.Client()}
			r := httptest.NewRequest("POST", tc.path, nil)
			r.Form = url.Values{"id": {"node"}, "revision": {"7"}, "index": {"0"}, "name": {"edited"}, "check_id": {"backup"}, "label": {"edited"}, "interval": {"48"}, "grace": {"3"}}
			if _, err := u.mutate(r.Context(), c, r, map[string]any{}); err != nil {
				t.Fatal(err)
			}
			rows, _ := saved[tc.field].([]any)
			want := 2
			if tc.remove {
				want = 1
			}
			if len(rows) != want {
				t.Fatalf("expected %d rows, got %v", want, rows)
			}
			if !tc.remove {
				v := rows[0].(map[string]any)
				if v["last_success_at"] != original["last_success_at"] || v["observed_at"] != original["observed_at"] {
					t.Fatal("lost check evidence", v)
				}
				if tc.field == "services" && v["name"] != "edited" {
					t.Fatal(v)
				}
				if tc.field == "backups" && (v["label"] != "edited" || v["interval_hours"] != float64(48)) {
					t.Fatal(v)
				}
			}
			if rows[len(rows)-1].(map[string]any)["name"] != "untouched" {
				t.Fatal(rows)
			}
		})
	}
}

func TestNodeMetadataEditPreservesOtherLabels(t *testing.T) {
	var saved map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Write([]byte(`{"id":"node","revision":3,"labels":{"owner":"old","custom":"keep"},"links":{"old":"https://old.invalid"}}`))
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
			t.Error(err)
		}
		w.Write([]byte(`{"id":"node"}`))
	}))
	defer api.Close()
	u := New(api.URL, api.Client(), false, time.Now)
	c := &apiclient.Client{Base: api.URL, HTTP: api.Client()}
	r := httptest.NewRequest("POST", "/nodes/save", nil)
	r.Form = url.Values{"id": {"node"}, "revision": {"3"}, "name": {"host"}, "owner": {"new"}, "provider": {"cloud"}, "location": {"west"}, "links": {"Panel = https://panel.invalid/?q=a=b\nDocs = https://docs.invalid"}}
	if _, err := u.mutate(r.Context(), c, r, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	labels := saved["labels"].(map[string]any)
	links := saved["links"].(map[string]any)
	if labels["owner"] != "new" || labels["custom"] != "keep" || labels["provider"] != "cloud" || links["Panel"] != "https://panel.invalid/?q=a=b" || len(links) != 2 {
		t.Fatal(labels, links)
	}
}
