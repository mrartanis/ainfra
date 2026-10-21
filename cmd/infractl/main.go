package main

import (
	"ainfra/internal/apiclient"
	"ainfra/internal/cli"
	"context"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	args := os.Args[1:]
	config := os.Getenv("INFRA_CONFIG")
	filtered := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--config" && i+1 < len(args) {
			i++
			config = args[i]
		} else {
			filtered = append(filtered, args[i])
		}
	}
	if len(filtered) == 0 || filtered[0] == "--help" {
		fmt.Print(cli.Help)
		return nil
	}
	if filtered[0] == "version" {
		fmt.Println("infractl 0.1.0")
		return nil
	}
	var c struct {
		Endpoint  string `json:"endpoint"`
		TokenFile string `json:"token_file"`
		CAFile    string `json:"ca_file"`
	}
	b, e := os.ReadFile(config)
	if e != nil {
		return fmt.Errorf("set --config or INFRA_CONFIG")
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return e
	}
	if e = apiclient.ValidateURL(c.Endpoint, true); e != nil {
		return e
	}
	secret, e := apiclient.ReadSecret(c.TokenFile)
	if e != nil {
		return e
	}
	h, e := apiclient.HTTP(c.CAFile)
	if e != nil {
		return e
	}
	return cli.Run(context.Background(), &apiclient.Client{Base: c.Endpoint, Token: secret, HTTP: h}, filtered, os.Stdout)
}
