package main

import (
	"ainfra/internal/adapters/feeds"
	"ainfra/internal/adapters/httpapi"
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/agent"
	"ainfra/internal/apiclient"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		log.Print(e)
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
		return store.Backup(ctx, *output)
	case "prune":
		result, err := store.Prune(ctx, policy, time.Now())
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "sync-feeds":
		return syncFeeds(ctx, svc)
	case "serve":
		ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
		if _, err := store.Prune(ctx, policy, time.Now()); err != nil {
			return err
		}
		go func() {
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if _, err := store.Prune(ctx, policy, time.Now()); err != nil {
						log.Print("retention cleanup failed")
					}
				}
			}
		}()
		if *enableFeeds {
			go func() {
				for {
					if err := syncFeeds(ctx, svc); err != nil {
						log.Print("feed sync failed; previous snapshot retained")
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Hour):
					}
				}
			}()
		}
		server := &http.Server{Addr: *listen, Handler: httpapi.New(svc), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			server.Shutdown(shutdown)
		}()
		log.Printf("API listening on http://%s", *listen)
		e = server.ListenAndServe()
		if e == http.ErrServerClosed {
			return nil
		}
		return e
	default:
		return fmt.Errorf("commands: serve, bootstrap, create-node, backup, version")
	}
}

func syncFeeds(ctx context.Context, svc *app.Service) error {
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
		snapshot, err := source.Fetch(ctx, prev)
		if snapshot.Unchanged && err == nil {
			err = svc.FeedUnchanged(ctx, source.ID)
		} else {
			err = svc.SyncFeed(ctx, source.ID, snapshot.Records, snapshot.ETag, snapshot.Modified, err)
		}
		if err != nil {
			result = err
		}
	}
	return result
}
