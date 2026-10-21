package httpapi

import (
	"ainfra/internal/domain"
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func FuzzReportDecoding(f *testing.F) {
	f.Add([]byte(`{"schema_version":1,"report_id":"r","node_id":"n","agent_instance_id":"i","sequence":1,"observed_at":"2026-10-21T12:00:00Z","collectors":{}}`))
	f.Add([]byte(`{"sequence":-1}`))
	f.Add([]byte(`{} {}`))
	f.Add([]byte(`{"password":"synthetic-secret"}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 64<<10 {
			t.Skip()
		}
		var report domain.Report
		if err := decode(httptest.NewRecorder(), httptest.NewRequest("POST", "/", bytes.NewReader(raw)), &report); err != nil {
			return
		}
		if !json.Valid(raw) {
			t.Fatal("invalid JSON accepted")
		}
		encoded, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		var again domain.Report
		if err = decode(httptest.NewRecorder(), httptest.NewRequest("POST", "/", bytes.NewReader(encoded)), &again); err != nil {
			t.Fatal("decoded report cannot round-trip", err)
		}
	})
}
