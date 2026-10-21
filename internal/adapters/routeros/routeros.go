package routeros

import (
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Collector struct {
	http                 *http.Client
	base, user, password string
	now                  func() time.Time
}

func New(client *http.Client, base, user, password string, now func() time.Time) *Collector {
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Collector{&copy, strings.TrimRight(base, "/"), user, password, now}
}
func (c *Collector) get(ctx context.Context, path, fields string) ([]map[string]string, error) {
	u, e := url.Parse(c.base)
	if e != nil || u.Scheme != "https" || u.User != nil {
		return nil, errors.New("HTTPS required")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", c.base+"/rest/"+path+"?.proplist="+url.QueryEscape(fields), nil)
	if e != nil {
		return nil, e
	}
	req.SetBasicAuth(c.user, c.password)
	resp, e := c.http.Do(req)
	if e != nil {
		return nil, errors.New("probe_connection")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("probe_http")
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if e != nil || len(b) > 2<<20 {
		return nil, errors.New("probe_limit")
	}
	var rows []map[string]string
	if e = json.Unmarshal(b, &rows); e != nil {
		return nil, errors.New("probe_format")
	}
	allowed := strings.Split(fields, ",")
	for _, r := range rows {
		for key := range r {
			ok := false
			for _, a := range allowed {
				if key == a {
					ok = true
				}
			}
			if !ok {
				delete(r, key)
			}
		}
	}
	return rows, nil
}
func (c *Collector) section(v any) domain.Section {
	b, _ := json.Marshal(v)
	return domain.Section{Status: "ok", Complete: true, ObservedAt: c.now(), Data: b}
}
func (c *Collector) Collect(ctx context.Context, _ []string) map[string]domain.Section {
	out := map[string]domain.Section{}
	resources, e := c.get(ctx, "system/resource", "version,architecture-name,board-name,cpu-count,cpu-load,total-memory,free-memory,uptime,total-hdd-space,free-hdd-space")
	if e != nil || len(resources) != 1 {
		out["os"] = domain.Section{Status: "error", ErrorCode: "resource_unavailable"}
		return out
	}
	r := resources[0]
	if !strings.HasPrefix(r["version"], "7.") {
		out["os"] = domain.Section{Status: "unsupported", ErrorCode: "routeros_version"}
		return out
	}
	cpu, e1 := strconv.Atoi(r["cpu-count"])
	mem, e2 := strconv.ParseUint(r["total-memory"], 10, 64)
	if e1 != nil || e2 != nil {
		out["os"] = domain.Section{Status: "error", ErrorCode: "resource_format"}
		return out
	}
	os := domain.OSData{ID: "routeros", Version: r["version"], Arch: r["architecture-name"], CPU: cpu, MemoryBytes: mem, Reboot: "unknown"}
	identity, e := c.get(ctx, "system/identity", "name")
	if e == nil && len(identity) == 1 {
		os.Hostname = identity[0]["name"]
	}
	out["os"] = c.section(os)
	endpoints := []struct{ section, path, fields string }{{"network", "interface", "name,type,mtu,running,disabled"}, {"network", "ip/address", "address,network,interface,disabled"}, {"network", "ip/route", "dst-address,gateway,distance,active"}, {"network", "ipv6/address", "address,interface,disabled"}, {"network", "ipv6/route", "dst-address,gateway,distance,active"}, {"wireguard", "interface/wireguard", "name,public-key,listen-port,mtu,disabled"}, {"wireguard", "interface/wireguard/peers", "interface,public-key,endpoint-address,endpoint-port,allowed-address,last-handshake,rx,tx,disabled"}}
	data := map[string]map[string]any{"network": {}, "wireguard": {}}
	bad := map[string]bool{}
	for _, ep := range endpoints {
		rows, e := c.get(ctx, ep.path, ep.fields)
		if e != nil {
			bad[ep.section] = true
			continue
		}
		normalized := []map[string]any{}
		for _, row := range rows {
			n := map[string]any{}
			for key, value := range row {
				switch key {
				case "disabled", "running", "active":
					if value != "true" && value != "false" {
						bad[ep.section] = true
						continue
					}
					n[key] = value == "true"
				case "mtu", "distance", "listen-port", "endpoint-port", "rx", "tx":
					v, e := strconv.ParseUint(value, 10, 64)
					if e != nil {
						bad[ep.section] = true
						continue
					}
					n[key] = v
				default:
					n[key] = value
				}
			}
			normalized = append(normalized, n)
		}
		data[ep.section][ep.path] = normalized
	}
	for name, v := range data {
		s := c.section(v)
		if bad[name] {
			s.Status = "partial"
			s.Complete = false
			s.ErrorCode = "endpoint_unavailable_or_format"
		}
		out[name] = s
	}
	out["packages"] = domain.Section{Status: "unsupported", ErrorCode: "routeros_package_cve_not_covered"}
	return out
}
