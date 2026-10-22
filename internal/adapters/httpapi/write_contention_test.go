package httpapi_test

import (
	"ainfra/internal/adapters/httpapi"
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDocumentWriteDuringLongWriterIsBoundedAndNeverRunsLate(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := app.New(store, time.Now, app.RandomID)
	ctx := context.Background()
	admin := domain.Principal{Role: "admin"}
	doc, err := svc.EditDocument(ctx, admin, domain.Document{Title: "Journal", Kind: "note", Body: "original"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := svc.IssueToken(ctx, admin, domain.Principal{Role: "editor"})
	if err != nil {
		t.Fatal(err)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- store.Write(ctx, func(tx app.Tx) error {
			close(entered)
			<-release
			return tx.Put("feed", "synthetic", domain.Feed{ID: "synthetic"})
		})
	}()
	<-entered
	// Always release on test failure, before store.Close.
	released := false
	defer func() {
		if !released {
			close(release)
			<-done
		}
	}()
	h := httpapi.New(svc)
	patch := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("PATCH", "/api/v1/documents/"+doc.ID, strings.NewReader(`{"title":"Journal","kind":"note","body":"updated"}`))
		deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		r = r.WithContext(deadline)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("If-Match", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	start := time.Now()
	w := patch()
	if w.Code != 503 || w.Header().Get("Retry-After") == "" || time.Since(start) > 4*time.Second {
		t.Fatalf("contention: %d %s after %s", w.Code, w.Body.String(), time.Since(start))
	}
	close(release)
	err = <-done
	released = true
	if err != nil {
		t.Fatal(err)
	}
	current, err := svc.Document(ctx, admin, doc.ID, 0)
	if err != nil || current.Revision != 1 || current.Body != "original" {
		t.Fatal("rejected request mutated document", current, err)
	}
	if w := patch(); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
