package ui

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNodeOverviewShowsObservedResourcesAndProvenance(t *testing.T) {
	u := New("http://unused", nil, false, time.Now)
	response := httptest.NewRecorder()
	u.render(response, map[string]any{"Page": "node", "Tab": "", "Node": map[string]any{"id": "node", "name": "Host", "observations": map[string]any{"os": map[string]any{"observed_at": "2026-10-21T12:00:00Z", "data": map[string]any{"id": "routeros", "version": "7.20", "model": "test-board", "cpu": 2, "memory_bytes": 1000}}, "disks": map[string]any{"observed_at": "2026-10-21T12:00:00Z", "data": []any{map[string]any{"path": "/srv", "filesystem": "ext4", "capacity_bytes": 4096, "available_bytes": 1024}}}}}})
	text := response.Body.String()
	for _, value := range []string{"test-board", "routeros", "7.20", "/srv", "ext4", "2026-10-21T12:00:00Z"} {
		if !strings.Contains(text, value) {
			t.Fatalf("missing resource %s", value)
		}
	}
}

func TestNodeOverviewSeparatesObservedInvoiceFromManualSchedule(t *testing.T) {
	u := New("http://unused", nil, false, time.Now)
	w := httptest.NewRecorder()
	u.render(w, map[string]any{"Page": "node", "Node": map[string]any{"id": "node", "name": "billing", "payment": map[string]any{"next_due": "2026-12-01", "amount": "20", "currency": "EUR"}, "payment_checks": map[string]any{"invoice": map[string]any{"observed_at": "2026-10-21T12:00:00Z", "last_success_at": "2026-10-20T12:00:00Z", "status": "error", "next_due": "2026-11-01", "amount": "10", "currency": "EUR", "evidence": "invoice evidence"}}}})
	text := w.Body.String()
	for _, want := range []string{"Ручной график оплаты", "Наблюдаемая оплата", "2026-12-01", "2026-11-01", "invoice evidence", "error", "2026-10-20T12:00:00Z"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s", want)
		}
	}
}
