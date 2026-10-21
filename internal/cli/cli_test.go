package cli_test

import (
	"ainfra/internal/apiclient"
	"ainfra/internal/cli"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
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
