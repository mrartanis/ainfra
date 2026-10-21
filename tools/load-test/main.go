// Local opt-in workload. Run in a container with --cpus 2 --memory 2g.
package main

import (
	"ainfra/internal/adapters/httpapi"
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"sync/atomic"
	"time"
)

func main() {
	path := flag.String("db", ".local/load.db", "disposable database, optionally restored with real feeds")
	nodes := flag.Int("nodes", 100, "node count")
	docs := flag.Int("docs", 10000, "document count")
	reuse := flag.Bool("reuse", false, "reuse an existing seeded benchmark database for read measurements")
	publish := flag.String("publish-feed", "", "republish a cached real feed concurrently with read measurements (disposable database only)")
	flag.Parse()
	if e := run(*path, *nodes, *docs, *reuse, *publish); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(path string, nodeCount, docCount int, reuse bool, publish string) error {
	ctx := context.Background()
	store, e := sqlite.Open(path)
	if e != nil {
		return e
	}
	defer store.Close()
	svc := app.New(store, time.Now, app.RandomID)
	admin := domain.Principal{ID: "load-test", Role: "admin"}
	existing, e := svc.Nodes(ctx, admin)
	if e != nil {
		return e
	}
	if len(existing) > 0 && !reuse {
		return fmt.Errorf("load database must not contain nodes")
	}
	started := time.Now()
	if reuse && len(existing) != nodeCount {
		return fmt.Errorf("expected %d seeded nodes, got %d", nodeCount, len(existing))
	}
	if !reuse {
		packages := make([]domain.Package, 5000)
		for i := range packages {
			packages[i] = domain.Package{Name: fmt.Sprintf("fixture-package-%04d", i), Arch: "amd64", Version: "1.0-1", Source: fmt.Sprintf("fixture-source-%04d", i), SourceVersion: "1.0-1", State: "installed", BlockedReason: "unknown"}
		}
		for i, source := range []string{"openssl", "curl", "bash"} {
			packages[i] = domain.Package{Name: source, Arch: "amd64", Version: "1.0-1", Source: source, SourceVersion: "1.0-1", State: "installed", Origin: "Ubuntu", OriginEvidence: "synthetic load fixture", Suite: "jammy"}
		}
		packageData, _ := json.Marshal(domain.PackageData{Packages: packages, MetadataStatus: "unknown"})
		for i := 0; i < nodeCount; i++ {
			n, e := svc.SaveNode(ctx, admin, domain.Node{Name: fmt.Sprintf("load-node-%03d", i), Purpose: "synthetic benchmark", Kind: "vm"}, 0)
			if e != nil {
				return e
			}
			now := time.Now()
			r := domain.Report{SchemaVersion: 1, ID: app.RandomID(), NodeID: n.ID, Instance: "load", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"os": {Status: "ok", Complete: true, ObservedAt: now, Data: json.RawMessage(`{"id":"ubuntu","version":"22.04","codename":"jammy"}`)}, "packages": {Status: "ok", Complete: true, ObservedAt: now, Data: packageData}}}
			if _, e = svc.IngestReport(ctx, domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "load"}, r); e != nil {
				return e
			}
		}
		for i := 0; i < docCount; i++ {
			if _, e = svc.EditDocument(ctx, admin, domain.Document{Title: fmt.Sprintf("Инструкция %05d", i), Kind: "runbook", Body: "Синтетическая инструкция для измерения поиска и размера хранилища. Backup restore проверяется отдельно."}, 0); e != nil {
				return e
			}
		}
	}
	fmt.Fprintf(os.Stderr, "seed completed in %s\n", time.Since(started))
	token, _, e := svc.IssueToken(ctx, admin, domain.Principal{Role: "reader"})
	if e != nil {
		return e
	}
	server := httptest.NewServer(httpapi.New(svc))
	defer server.Close()
	results := map[string]any{"nodes": nodeCount, "packages_per_node": 5000, "documents": docCount, "go": runtime.Version(), "seed_seconds": time.Since(started).Seconds(), "reused_seed": reuse}
	type publicationResult struct {
		err     error
		seconds float64
	}
	var publication chan publicationResult
	var publishing atomic.Bool
	if publish != "" {
		var feed domain.Feed
		if err := store.Read(ctx, func(tx app.Tx) error { return tx.Get("feed", publish, &feed) }); err != nil {
			return err
		}
		publication = make(chan publicationResult, 1)
		entered := make(chan struct{})
		go func() {
			began := time.Now()
			snapshot := cachedSnapshot{store: store, ctx: ctx, id: publish, entered: entered}
			publishing.Store(true)
			err := svc.SyncSnapshot(ctx, publish, snapshot, feed.ETag, feed.SourceModified, nil)
			publishing.Store(false)
			publication <- publicationResult{err: err, seconds: time.Since(began).Seconds()}
		}()
		select {
		case <-entered:
		case result := <-publication:
			return fmt.Errorf("publication failed before snapshot: %v", result.err)
		}
	}
	for _, path := range []string{"nodes", "attention", "search?q=Backup"} {
		samples := []float64{}
		var size int64
		concurrent := 0
		for i := 0; i < 55; i++ {
			req, _ := http.NewRequest("GET", server.URL+"/api/v1/"+path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			duringPublication := publishing.Load()
			start := time.Now()
			resp, e := server.Client().Do(req)
			if e != nil {
				return e
			}
			size, e = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if e != nil {
				return e
			}
			if resp.StatusCode != 200 {
				return fmt.Errorf("%s HTTP %d", path, resp.StatusCode)
			}
			if duringPublication && publishing.Load() {
				concurrent++
			}
			if i >= 5 {
				samples = append(samples, float64(time.Since(start).Microseconds())/1000)
			}
		}
		sort.Float64s(samples)
		results[path] = map[string]any{"p50_ms": samples[len(samples)/2], "p95_ms": samples[int(float64(len(samples))*.95)-1], "response_bytes": size, "requests_during_publication": concurrent}
	}
	if publication != nil {
		result := <-publication
		if result.err != nil {
			return result.err
		}
		results["publication"] = map[string]any{"feed": publish, "seconds": result.seconds}
	}
	status, _ := os.ReadFile("/proc/self/status")
	results["process_status"] = string(status)
	return json.NewEncoder(os.Stdout).Encode(results)
}

// Stream the committed cached snapshot through the production publication path.
// It uses the independent read pool while the writer owns its transaction.
type cachedSnapshot struct {
	store   *sqlite.Store
	ctx     context.Context
	id      string
	entered chan struct{}
}

func (s cachedSnapshot) Walk(visit func(string, []domain.Advisory) error) error {
	close(s.entered)
	return s.store.Read(s.ctx, func(tx app.Tx) error {
		var sources []string
		if err := tx.Get("feed-sources", s.id, &sources); err != nil {
			return err
		}
		for _, source := range sources {
			var rows []domain.Advisory
			if err := tx.Get("advisory:"+s.id, source, &rows); err != nil {
				return err
			}
			if err := visit(source, rows); err != nil {
				return err
			}
		}
		return nil
	})
}
