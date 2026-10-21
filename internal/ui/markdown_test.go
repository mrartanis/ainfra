package ui

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDocumentPreviewFormatsMarkdownWithoutActiveHTML(t *testing.T) {
	u := New("http://unused", nil, false, time.Now)
	w := httptest.NewRecorder()
	u.render(w, map[string]any{"Page": "document", "Document": map[string]any{"body": "# Heading\n\n**Strong**\n\n<script>alert(1)</script>\n\n[bad](javascript:alert%281%29)\n\n<iframe src='https://evil.invalid'></iframe>", "kind": "note"}})
	html := w.Body.String()
	if !strings.Contains(html, "<h1>Heading</h1>") || !strings.Contains(html, "<strong>Strong</strong>") {
		t.Fatal("Markdown preview missing")
	}
	for _, bad := range []string{"<script", "<iframe", `href="javascript:`} {
		if strings.Contains(html, bad) {
			t.Fatalf("unsafe preview: %s", bad)
		}
	}
}

func TestPreviewDoesNotWriteAPIAndPreservesRevision(t *testing.T) {
	u := New("http://unused", nil, false, time.Now)
	r := httptest.NewRequest("POST", "/docs/preview", strings.NewReader("id=doc&revision=7&title=Title&kind=note&body=%23+Draft&nodes=node"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	data := map[string]any{}
	dest, err := u.mutate(r.Context(), nil, r, data)
	if err != nil || dest != "" || data["Preview"] != true {
		t.Fatal(dest, err, data)
	}
	doc := data["Document"].(map[string]any)
	if doc["revision"] != 7 || doc["body"] != "# Draft" {
		t.Fatal(doc)
	}
}
