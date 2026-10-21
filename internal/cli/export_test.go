package cli_test

import (
	"ainfra/internal/apiclient"
	"ainfra/internal/cli"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExportTraversesPagesAndFetchesFullNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/nodes":
			id := "a"
			if r.URL.Query().Get("offset") == "0" {
				w.Header().Set("X-Next-Offset", "1")
			} else {
				id = "b"
			}
			fmt.Fprintf(w, `[{"id":%q,"slug":%q}]`, id, id)
		case "/api/v1/nodes/a", "/api/v1/nodes/b":
			fmt.Fprint(w, `{"observations":{"packages":{"data":{"packages":[{"name":"full-inventory"}]}}}}`)
		case "/api/v1/documents":
			fmt.Fprint(w, `[{"id":"doc","title":"Instruction","body":"backup first"}]`)
		case "/api/v1/relations", "/api/v1/events":
			fmt.Fprint(w, `[]`)
		default:
			t.Error("unexpected API call", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c := &apiclient.Client{Base: server.URL, Token: "test", HTTP: server.Client()}
	dest := filepath.Join(t.TempDir(), "export")
	var output bytes.Buffer
	if e := cli.Run(context.Background(), c, []string{"export", "--output", dest}, &output); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"a", "b"} {
		raw, e := os.ReadFile(filepath.Join(dest, "nodes", id, "node.json"))
		if e != nil || !bytes.Contains(raw, []byte("full-inventory")) {
			t.Fatal(id, string(raw), e)
		}
	}
	if _, e := os.Stat(filepath.Join(dest, "documents/doc.md")); e != nil {
		t.Fatal(e)
	}
}
