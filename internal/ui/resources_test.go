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
