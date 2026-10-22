package cli_test

import (
	"ainfra/internal/apiclient"
	"ainfra/internal/cli"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIReadsAPIAndJSONIsClean(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes" || r.Header.Get("Authorization") != "Bearer test" {
			t.Error(r.URL.Path)
		}
		w.Write([]byte(`[{"id":"abc","name":"host"}]`))
	}))
	defer server.Close()
	var out bytes.Buffer
	c := &apiclient.Client{Base: server.URL, Token: "test", HTTP: server.Client()}
	if e := cli.Run(context.Background(), c, []string{"nodes", "list", "--json"}, &out); e != nil {
		t.Fatal(e)
	}
	if out.String() != "[\n  {\n    \"id\": \"abc\",\n    \"name\": \"host\"\n  }\n]\n" {
		t.Fatal(out.String())
	}
}

func TestCLIViewsAndJournalAppend(t *testing.T) {
	entryFile := filepath.Join(t.TempDir(), "entry.json")
	if err := os.WriteFile(entryFile, []byte(`{"id":"retry-id","step":"Validate","status":"succeeded","result":"exit 0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args               []string
		path, method, view string
	}{
		{[]string{"nodes", "list"}, "/api/v1/nodes", "GET", "brief"},
		{[]string{"nodes", "list", "--view", "summary"}, "/api/v1/nodes", "GET", "summary"},
		{[]string{"nodes", "show", "node"}, "/api/v1/nodes/node", "GET", "summary"},
		{[]string{"nodes", "show", "node", "--full"}, "/api/v1/nodes/node", "GET", "full"},
		{[]string{"docs", "entries", "doc", "--page-info"}, "/api/v1/documents/doc/entries", "GET", ""},
		{[]string{"docs", "append", "doc", "--file", entryFile}, "/api/v1/documents/doc/entries", "POST", ""},
	} {
		t.Run(tc.path+tc.view+tc.method, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != tc.path || r.Method != tc.method || r.URL.Query().Get("view") != tc.view {
					t.Error(r.Method, r.URL)
				}
				if tc.method == "POST" {
					var entry map[string]any
					if err := json.NewDecoder(r.Body).Decode(&entry); err != nil || entry["id"] != "retry-id" {
						t.Error(entry, err)
					}
				}
				w.Header().Set("X-Next-Offset", "10")
				w.Write([]byte(`{}`))
			}))
			defer server.Close()
			var out bytes.Buffer
			client := &apiclient.Client{Base: server.URL, HTTP: server.Client()}
			if err := cli.Run(context.Background(), client, tc.args, &out); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatal(calls)
			}
		})
	}
}
