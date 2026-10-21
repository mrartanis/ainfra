package httpapi_test

import (
	"ainfra/internal/adapters/httpapi"
	"ainfra/internal/app"
	"ainfra/internal/testkit"
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStructuredRequestLogCorrelatesWithoutCredentials(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	handler := httpapi.NewWithLogger(app.New(testkit.NewStore(), time.Now, testkit.IDs()), logger)
	req := httptest.NewRequest("GET", "/api/v1/nodes/secret_path?token=secret_query", strings.NewReader("secret_body"))
	req.Header.Set("Authorization", "Bearer secret_auth")
	req.Header.Set("X-Request-ID", "secret_supplied_id")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["status"] != float64(401) || entry["request_id"] != w.Header().Get("X-Request-ID") || entry["method"] != "GET" {
		t.Fatal(entry)
	}
	if strings.Contains(output.String(), "secret_") {
		t.Fatal("request data leaked into logs")
	}
}
