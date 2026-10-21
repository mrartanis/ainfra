package main

import (
	"ainfra/internal/adapters/linuxcollect"
	"ainfra/internal/adapters/routeros"
	"ainfra/internal/agent"
	"ainfra/internal/apiclient"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("commands: collect, push, run, check-config, version")
	}
	cmd := os.Args[1]
	f := flag.NewFlagSet("infra-agent "+cmd, flag.ContinueOnError)
	config := f.String("config", "/etc/infra/agent.json", "config JSON")
	file := f.String("file", "", "report file")
	f.Bool("json", false, "JSON output")
	if e := f.Parse(os.Args[2:]); e != nil {
		return e
	}
	if cmd == "version" {
		fmt.Println("infra-agent 0.1.0")
		return nil
	}
	c, e := agent.LoadConfig(*config)
	if e != nil {
		return e
	}
	if cmd == "check-config" {
		fmt.Println("ok")
		return nil
	}
	h, e := apiclient.HTTP(c.CAFile)
	if e != nil {
		return e
	}
	secret, e := apiclient.ReadSecret(c.TokenFile)
	if e != nil {
		return e
	}
	transport := agent.APITransport{Client: &apiclient.Client{Base: c.Server, Token: secret, HTTP: h}}
	collector := linuxcollect.New(linuxcollect.ExecRunner{}, os.ReadFile, time.Now)
	a := agent.New(c, collector, transport, time.Now, app.RandomID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	switch cmd {
	case "probe":
		if len(c.ProbeTargets) == 0 {
			return fmt.Errorf("no probe targets")
		}
		for _, target := range c.ProbeTargets {
			if e = apiclient.ValidateURL(target.URL, false); e != nil {
				return e
			}
			raw, e := apiclient.ReadSecret(target.CredentialFile)
			if e != nil {
				return e
			}
			var credentials struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if e = json.Unmarshal([]byte(raw), &credentials); e != nil {
				return fmt.Errorf("invalid credential file")
			}
			client, e := apiclient.HTTP(target.CAFile)
			if e != nil {
				return e
			}
			secret, e := apiclient.ReadSecret(target.TokenFile)
			if e != nil {
				return e
			}
			cfg := c
			cfg.NodeID = target.NodeID
			cfg.StateDir = filepath.Join(c.StateDir, target.NodeID)
			cfg.CheckFiles = nil
			probe := routeros.New(client, target.URL, credentials.Username, credentials.Password, time.Now)
			sender := agent.APITransport{Client: &apiclient.Client{Base: c.Server, Token: secret, HTTP: h}}
			if e = agent.New(cfg, probe, sender, time.Now, app.RandomID).Run(ctx); e != nil {
				return e
			}
		}
		return nil
	case "collect":
		r, e := a.Collect(ctx)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(r)
	case "run":
		return a.Run(ctx)
	case "push":
		b, e := os.ReadFile(*file)
		if e != nil {
			return e
		}
		if len(b) > 16<<20 {
			return fmt.Errorf("report too large")
		}
		var r domain.Report
		if e = json.Unmarshal(b, &r); e != nil {
			return e
		}
		return transport.Send(ctx, r)
	default:
		return fmt.Errorf("unknown command")
	}
}
