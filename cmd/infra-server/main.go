package main

import (
	"ainfra/internal/adapters/feeds"
	"ainfra/internal/adapters/httpapi"
	"ainfra/internal/adapters/reportinbox"
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/agent"
	"ainfra/internal/apiclient"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if e := run(); e != nil {
		slog.Error("command failed", "error", e.Error())
		os.Exit(1)
	}
}
func run() error {
	command := "serve"
	args := os.Args[1:]
	if len(args) > 0 && args[0][0] != '-' {
		command = args[0]
		args = args[1:]
	}
	f := flag.NewFlagSet("infra-server "+command, flag.ContinueOnError)
	db := f.String("db", ".local/infra.db", "SQLite path")
	inboxPath := f.String("inbox-db", "", "durable report inbox SQLite (default DB path + .inbox)")
	listen := f.String("listen", "127.0.0.1:8080", "HTTP listen (TLS terminates at proxy)")
	output := f.String("output", "", "private output file for bootstrap token or backup")
	role := f.String("role", "admin", "bootstrap token role")
	node := f.String("node", "", "target node ID")
	instance := f.String("instance", "", "agent instance UUID")
	retentionDays := f.Int("retention-days", 90, "receipt/event/resolved finding retention days")
	maxHistory := f.Int("max-history", 100000, "maximum receipts and events each")
	documentRevisions := f.Int("document-revisions", 20, "latest document revisions, plus approved")
	maxDB := f.Int("max-db-mib", 1024, "maximum SQLite database size MiB")
	enableFeeds := f.Bool("feeds", true, "sync official CVE feeds hourly")
	name := f.String("name", "", "create node name")
	if e := f.Parse(args); e != nil {
		return e
	}
	if command == "version" {
		fmt.Println("infra-server 0.1.0")
		return nil
	}
	store, e := sqlite.Open(*db)
	if e != nil {
		return e
	}
	defer store.Close()
	policy := sqlite.Retention{Days: *retentionDays, MaxHistory: *maxHistory, DocumentRevisions: *documentRevisions, MaxDatabaseMiB: *maxDB}
	if e = store.SetSizeLimit(context.Background(), *maxDB); e != nil {
		return e
	}
	svc := app.New(store, time.Now, app.RandomID)
	ctx := context.Background()
	admin := domain.Principal{ID: "local-admin", Role: "admin"}
	switch command {
	case "bootstrap":
		if *output == "" {
			return fmt.Errorf("--output token file required")
		}
		if _, e = os.Stat(*output); !os.IsNotExist(e) {
			return fmt.Errorf("output exists")
		}
		p := domain.Principal{Role: *role, Instance: *instance}
		if *node != "" {
			p.Nodes = []string{*node}
		}
		secret, meta, e := svc.IssueToken(ctx, admin, p)
		if e != nil {
			return e
		}
		if e = agent.AtomicWrite(*output, []byte(secret+"\n")); e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(meta)
	case "create-node":
		n, e := svc.SaveNode(ctx, admin, domain.Node{Name: *name}, 0)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(n)
	case "backup":
		if *output == "" {
			return fmt.Errorf("--output required")
		}
		if _, err := os.Stat(*output); !os.IsNotExist(err) {
			return fmt.Errorf("output exists or inaccessible")
		}
		if *inboxPath == "" {
			*inboxPath = *db + ".inbox"
		}
		if _, err := os.Stat(*inboxPath); err == nil {
			// Inbox first: processing rows replay safely against a later inventory snapshot.
			if err := reportinbox.Backup(ctx, *inboxPath, *output+".inbox"); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		return store.Backup(ctx, *output)
	case "prune":
		result, err := store.Prune(ctx, policy, time.Now())
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "sync-feeds":
		return syncFeeds(ctx, svc, filepath.Join(filepath.Dir(*db), "feed-staging"))
	case "serve":
		if *inboxPath == "" {
			*inboxPath = *db + ".inbox"
		}
		inbox, err := reportinbox.Open(*inboxPath)
		if err != nil {
			return err
		}
		defer inbox.Close()
		ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
		if _, err := store.Prune(ctx, policy, time.Now()); err != nil {
			return err
		}

		listener, err := net.Listen("tcp", *listen)
		if err != nil {
			return err
		}
		defer listener.Close()
		var workers sync.WaitGroup
		defer func() { stop(); workers.Wait() }()
		workers.Add(1)
		go func() { defer workers.Done(); inbox.Run(ctx, svc.IngestReport, slog.Default()) }()
		workers.Add(1)
		go func() {
			defer workers.Done()
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if _, err := store.Prune(ctx, policy, time.Now()); err != nil {
						slog.Error("retention cleanup failed")
					}
				}
			}
		}()
		if *enableFeeds {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for {
					if err := syncFeeds(ctx, svc, filepath.Join(filepath.Dir(*db), "feed-staging")); err != nil {
						slog.Error("feed sync failed; previous snapshot retained")
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Hour):
					}
				}
			}()
		}
		server := &http.Server{Addr: *listen, Handler: httpapi.NewWithInbox(svc, slog.Default(), inbox), ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelError), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
		slog.Info("API listening", "address", listener.Addr().String(), "transport", "http")
		return serveUntilCancelled(ctx, server, listener)
	default:
		return fmt.Errorf("commands: serve, bootstrap, create-node, backup, sync-feeds, prune, version")
	}
}

func syncFeeds(ctx context.Context, svc *app.Service, stagingRoot string) error {
	workspace, err := feeds.OpenWorkspace(stagingRoot)
	if err != nil {
		return err
	}
	defer workspace.Close()
	h, e := apiclient.HTTP("")
	if e != nil {
		return e
	}
	h.Timeout = 10 * time.Minute
	previous, e := svc.Feeds(ctx, domain.Principal{Role: "admin"})
	if e != nil {
		return e
	}
	var result error
	for _, source := range []feeds.Source{{ID: "debian", URL: feeds.DebianURL, HTTP: h}, {ID: "ubuntu", URL: feeds.UbuntuURL, HTTP: h}} {
		var prev domain.Feed
		for _, f := range previous {
			if f.ID == source.ID {
				prev = f
			}
		}
		stage, err := workspace.NewStage()
		if err != nil {
			return err
		}
		snapshot, err := source.FetchTo(ctx, prev, stage.Emit)
		if snapshot.Unchanged && err == nil {
			err = svc.FeedUnchanged(ctx, source.ID)
		} else {
			err = svc.SyncSnapshot(ctx, source.ID, stage, snapshot.ETag, snapshot.Modified, err)
		}
		closeErr := stage.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			result = err
		}
	}
	return result
}
