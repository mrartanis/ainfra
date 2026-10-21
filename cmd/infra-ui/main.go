package main

import (
	"ainfra/internal/apiclient"
	"ainfra/internal/ui"
	"flag"
	"log"
	"net/http"
	"time"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8081", "HTTP UI listen behind TLS proxy")
	api := flag.String("api", "http://127.0.0.1:8080", "API endpoint")
	dev := flag.Bool("dev-loopback", false, "non-secure session cookie for local HTTP testing only")
	ca := flag.String("ca-file", "", "API CA file")
	flag.Parse()
	if e := apiclient.ValidateURL(*api, true); e != nil {
		log.Fatal(e)
	}
	if *dev && !(len(*listen) > 10 && (*listen)[:10] == "127.0.0.1:") {
		log.Fatal("development mode requires literal IPv4 loopback")
	}
	h, e := apiclient.HTTP(*ca)
	if e != nil {
		log.Fatal(e)
	}
	s := &http.Server{Addr: *listen, Handler: ui.New(*api, h, *dev, time.Now), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	log.Printf("UI listening on http://%s", *listen)
	log.Fatal(s.ListenAndServe())
}
