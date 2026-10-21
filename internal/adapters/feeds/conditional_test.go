package feeds_test

import (
	"ainfra/internal/adapters/feeds"
	"ainfra/internal/domain"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestOldNormalizedFeedIsFetchedWithoutConditionalHeaders(t *testing.T) {
	fixture, err := os.ReadFile("../../../fixtures/feeds/debian-openssl.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{0, domain.FeedFormatVersion} {
		t.Run(map[bool]string{true: "current", false: "old"}[version == domain.FeedFormatVersion], func(t *testing.T) {
			current := version == domain.FeedFormatVersion
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if current {
					if r.Header.Get("If-None-Match") != "tag" || r.Header.Get("If-Modified-Since") != "modified" {
						t.Error("missing conditional headers")
					}
					w.WriteHeader(http.StatusNotModified)
				} else {
					if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
						t.Error("old normalization can remain cached forever")
					}
					w.Write(fixture)
				}
			}))
			defer server.Close()
			source := feeds.Source{ID: "debian", URL: server.URL, HTTP: server.Client()}
			result, err := source.FetchTo(context.Background(), domain.Feed{FormatVersion: version, ETag: "tag", SourceModified: "modified"}, func(domain.Advisory) error { return nil })
			if err != nil || result.Unchanged != current {
				t.Fatal(result, err)
			}
		})
	}
}
