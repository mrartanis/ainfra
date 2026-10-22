package linuxcollect

import (
	"ainfra/internal/apiclient"
	"ainfra/internal/domain"
	"context"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// WithWireProxy selects an explicitly configured userspace WireGuard source.
// Its local public key is provisioned separately; private config is never read.
func (c *Collector) WithWireProxy(metricsURL, publicKey string) *Collector {
	copy := *c
	copy.wireproxyURL, copy.wireproxyPublicKey = metricsURL, publicKey
	return &copy
}

func (c *Collector) wireProxy(ctx context.Context) domain.Section {
	key, err := base64.StdEncoding.DecodeString(c.wireproxyPublicKey)
	if apiclient.ValidateLocalMetricsURL(c.wireproxyURL) != nil || err != nil || len(key) != 32 {
		return failed("wireproxy_invalid_config")
	}
	request, err := http.NewRequestWithContext(ctx, "GET", c.wireproxyURL, nil)
	if err != nil {
		return failed("wireproxy_invalid_config")
	}
	transport := &http.Transport{DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return failed("wireproxy_unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return failed("wireproxy_unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return failed("wireproxy_invalid_metrics")
	}
	return c.parseWireProxy(string(raw))
}

func (c *Collector) parseWireProxy(raw string) domain.Section {
	fields := map[string]string{"public-key": "wireproxy\t" + c.wireproxyPublicKey + "\n", "peers": "", "endpoints": "", "allowed-ips": "", "latest-handshakes": ""}
	peer := ""
	count := 0
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.ContainsAny(value, "\r\t") {
			return failed("wireproxy_invalid_metrics")
		}
		switch name {
		case "private_key", "preshared_key":
			// Reject old/unsafe endpoints; even an unexpected secret never enters data.
			if value != "REDACTED" {
				return failed("wireproxy_unsafe_metrics")
			}
		case "public_key":
			key, err := hex.DecodeString(value)
			if err != nil || len(key) != 32 {
				return failed("wireproxy_invalid_metrics")
			}
			peer = base64.StdEncoding.EncodeToString(key)
			count++
			if count > 4096 {
				return failed("wireproxy_invalid_metrics")
			}
			fields["peers"] += "wireproxy\t" + peer + "\n"
		case "endpoint":
			if peer == "" || len(value) > 256 {
				return failed("wireproxy_invalid_metrics")
			}
			fields["endpoints"] += "wireproxy\t" + peer + "\t" + value + "\n"
		case "allowed_ip":
			if peer == "" {
				return failed("wireproxy_invalid_metrics")
			}
			if _, err := netip.ParsePrefix(value); err != nil {
				return failed("wireproxy_invalid_metrics")
			}
			fields["allowed-ips"] += "wireproxy\t" + peer + "\t" + value + "\n"
		case "last_handshake_time_sec":
			if peer == "" {
				return failed("wireproxy_invalid_metrics")
			}
			if _, err := strconv.ParseUint(value, 10, 64); err != nil {
				return failed("wireproxy_invalid_metrics")
			}
			fields["latest-handshakes"] += "wireproxy\t" + peer + "\t" + value + "\n"
		}
	}
	if count == 0 {
		return failed("wireproxy_invalid_metrics")
	}
	return c.section(fields)
}
