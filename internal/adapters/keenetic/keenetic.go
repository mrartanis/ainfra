// Package keenetic collects public operational state through HTTPS RCI.
// The only POST is session authentication; device configuration is never read.
package keenetic

import (
	"ainfra/internal/domain"
	"bytes"
	"context"
	"crypto/md5" // Required by Keenetic's challenge-response protocol.
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var errProbe = errors.New("keenetic_unavailable")

type Collector struct {
	http                 *http.Client
	base, user, password string
	now                  func() time.Time
	authFailed           bool
}

func New(client *http.Client, base, user, password string, now func() time.Time) *Collector {
	copy := *client
	copy.Jar, _ = cookiejar.New(nil)
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if copy.Timeout == 0 || copy.Timeout > 12*time.Second {
		copy.Timeout = 12 * time.Second
	}
	if tr, ok := copy.Transport.(*http.Transport); ok {
		tr = tr.Clone()
		tr.Proxy = nil
		copy.Transport = tr
	} else if copy.Transport == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.Proxy = nil
		copy.Transport = tr
	}
	return &Collector{http: &copy, base: strings.TrimRight(base, "/"), user: user, password: password, now: now}
}

func (c *Collector) request(ctx context.Context, path string, data any) (int, http.Header, []byte, error) {
	u, err := url.Parse(c.base)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return 0, nil, nil, errProbe
	}
	if tr, ok := c.http.Transport.(*http.Transport); ok && tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		return 0, nil, nil, errProbe
	}
	method := "GET"
	var body io.Reader
	if data != nil {
		if path != "/auth" {
			return 0, nil, nil, errProbe
		}
		method = "POST"
		raw, _ := json.Marshal(data)
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return 0, nil, nil, errProbe
	}
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, nil, errProbe
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return 0, nil, nil, errProbe
	}
	return resp.StatusCode, resp.Header, raw, nil
}

func (c *Collector) authenticate(ctx context.Context) error {
	status, headers, _, err := c.request(ctx, "/auth", nil)
	realm, challenge := headers.Get("X-NDM-Realm"), headers.Get("X-NDM-Challenge")
	if err != nil || status != 401 || realm == "" || challenge == "" {
		return errProbe
	}
	first := md5.Sum([]byte(c.user + ":" + realm + ":" + c.password))
	proof := sha256.Sum256([]byte(challenge + hex.EncodeToString(first[:])))
	status, _, _, err = c.request(ctx, "/auth", map[string]string{"login": c.user, "password": hex.EncodeToString(proof[:])})
	if err != nil || status != 200 {
		return errProbe
	}
	return nil
}

func rciError(v any) bool {
	switch value := v.(type) {
	case map[string]any:
		if value["status"] == "error" && value["code"] != nil {
			return true
		}
		for _, child := range value {
			if rciError(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if rciError(child) {
				return true
			}
		}
	}
	return false
}

func (c *Collector) get(ctx context.Context, path string, out any) error {
	status, _, raw, err := c.request(ctx, path, nil)
	if err == nil && status == 401 {
		if c.authFailed {
			return errProbe
		}
		if err = c.authenticate(ctx); err != nil {
			c.authFailed = true
			return err
		}
		status, _, raw, err = c.request(ctx, path, nil)
		if status == 401 {
			c.authFailed = true
		}
	}
	if err != nil || status != 200 {
		return errProbe
	}
	var value any
	if json.Unmarshal(raw, &value) != nil || rciError(value) || json.Unmarshal(raw, out) != nil {
		return errProbe
	}
	return nil
}

func (c *Collector) section(data any, partial bool) domain.Section {
	raw, _ := json.Marshal(data)
	s := domain.Section{Status: "ok", Complete: true, ObservedAt: c.now(), Data: raw}
	if partial {
		s.Status = "partial"
		s.Complete = false
		s.ErrorCode = "keenetic_incomplete"
	}
	return s
}
func failed() domain.Section {
	return domain.Section{Status: "error", ErrorCode: "keenetic_unavailable"}
}

func number(raw json.RawMessage) (float64, error) {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		s = string(raw)
	}
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, errProbe
	}
	return v, nil
}
func publicKey(key string) bool {
	raw, err := base64.StdEncoding.DecodeString(key)
	return err == nil && len(raw) == 32
}

type wireguard struct {
	PublicKey  string  `json:"public-key"`
	ListenPort *uint16 `json:"listen-port"`
	Status     string  `json:"status"`
	Peers      []struct {
		PublicKey     string          `json:"public-key"`
		Enabled       *bool           `json:"enabled"`
		LastHandshake json.RawMessage `json:"last-handshake"`
	} `json:"peer"`
}
type iface struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Description string     `json:"description"`
	State       string     `json:"state"`
	Link        string     `json:"link"`
	Connected   string     `json:"connected"`
	MTU         *int       `json:"mtu"`
	Address     string     `json:"address"`
	Mask        string     `json:"mask"`
	WireGuard   *wireguard `json:"wireguard"`
}

func (c *Collector) Collect(ctx context.Context, selected []string) map[string]domain.Section {
	c.authFailed = false
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	out := map[string]domain.Section{}
	wanted := map[string]bool{}
	for _, name := range selected {
		wanted[name] = true
	}
	if len(selected) == 0 {
		wanted = map[string]bool{"os": true, "network": true, "wireguard": true}
	}
	if wanted["os"] {
		var version struct {
			Title string `json:"title"`
			Arch  string `json:"arch"`
			Model string `json:"model"`
		}
		var system map[string]json.RawMessage
		if c.get(ctx, "/rci/show/version", &version) != nil || version.Title == "" || c.get(ctx, "/rci/show/system", &system) != nil || system == nil {
			out["os"] = failed()
		} else {
			data := map[string]any{"id": "keeneticos", "version": version.Title, "arch": version.Arch, "model": version.Model, "reboot": "unknown"}
			var hostname string
			if json.Unmarshal(system["hostname"], &hostname) == nil {
				data["hostname"] = hostname
			}
			partial := false
			for _, field := range []struct {
				source, target string
				scale          float64
			}{{"memtotal", "memory_bytes", 1024}, {"memfree", "memory_free_bytes", 1024}, {"cpuload", "cpu_load_percent", 1}, {"uptime", "uptime_seconds", 1}} {
				if raw, ok := system[field.source]; ok {
					value, err := number(raw)
					if err != nil || (field.source == "cpuload" && value > 100) || (field.scale == 1024 && (value != math.Trunc(value) || value >= float64(math.MaxUint64)/1024)) {
						partial = true
						continue
					}
					if field.scale == 1024 {
						data[field.target] = uint64(value) * 1024
					} else {
						data[field.target] = value
					}
				}
			}
			out["os"] = c.section(data, partial)
		}
	}
	if !wanted["network"] && !wanted["wireguard"] {
		return out
	}
	var interfaces map[string]iface
	if c.get(ctx, "/rci/show/interface", &interfaces) != nil || interfaces == nil {
		for _, name := range []string{"network", "wireguard"} {
			if wanted[name] {
				out[name] = failed()
			}
		}
		return out
	}
	ids := make([]string, 0, len(interfaces))
	for id := range interfaces {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if wanted["network"] {
		rows := []map[string]any{}
		partial := false
		for _, id := range ids {
			v := interfaces[id]
			if v.ID == "" {
				partial = true
				continue
			}
			row := map[string]any{"name": v.ID, "type": v.Type, "description": v.Description, "state": v.State, "link": v.Link}
			if v.MTU != nil && *v.MTU > 0 {
				row["mtu"] = *v.MTU
			}
			if v.Connected == "yes" || v.Connected == "no" {
				row["connected"] = v.Connected == "yes"
			}
			if v.Address != "" {
				addr, e := netip.ParseAddr(v.Address)
				if e != nil {
					partial = true
				} else {
					row["address"] = addr.String()
					if mask, e := netip.ParseAddr(v.Mask); e == nil {
						row["netmask"] = mask.String()
					}
				}
			}
			rows = append(rows, row)
		}
		data := map[string]any{"interfaces": rows}
		for _, ep := range []struct{ path, key string }{{"/rci/show/ip/route", "routes"}, {"/rci/show/ipv6/route", "routes_ipv6"}} {
			var raw json.RawMessage
			if c.get(ctx, ep.path, &raw) != nil {
				partial = true
				data[ep.key+"_status"] = "unknown"
				continue
			}
			routes, err := parseRoutes(raw)
			if err != nil {
				partial = true
				data[ep.key+"_status"] = "unknown"
				continue
			}
			data[ep.key] = routes
			data[ep.key+"_status"] = "ok"
		}
		out["network"] = c.section(data, partial)
	}
	if wanted["wireguard"] {
		fields := map[string]string{"public-key": "", "peers": "", "endpoints": "", "allowed-ips": "", "latest-handshakes": ""}
		rows := []map[string]any{}
		partial := false
		for _, id := range ids {
			v := interfaces[id]
			if v.Type != "Wireguard" {
				continue
			}
			if v.ID == "" || v.WireGuard == nil || !publicKey(v.WireGuard.PublicKey) {
				partial = true
				continue
			}
			wg := v.WireGuard
			fields["public-key"] += v.ID + "\t" + wg.PublicKey + "\n"
			row := map[string]any{"name": v.ID, "public_key": wg.PublicKey, "state": v.State}
			if wg.ListenPort != nil {
				row["listen_port"] = *wg.ListenPort
			}
			peers := []map[string]any{}
			for _, peer := range wg.Peers {
				if !publicKey(peer.PublicKey) {
					partial = true
					continue
				}
				p := map[string]any{"public_key": peer.PublicKey}
				if peer.Enabled != nil {
					p["enabled"] = *peer.Enabled
				}
				if peer.Enabled != nil && *peer.Enabled {
					fields["peers"] += v.ID + "\t" + peer.PublicKey + "\n"
				}
				if len(peer.LastHandshake) > 0 {
					age, err := number(peer.LastHandshake)
					if err != nil {
						partial = true
					} else if age < math.MaxInt32 {
						p["handshake_age_seconds"] = age
						at := c.now().Unix() - int64(age)
						if at > 0 {
							fields["latest-handshakes"] += v.ID + "\t" + peer.PublicKey + "\t" + strconv.FormatInt(at, 10) + "\n"
						}
					}
				}
				peers = append(peers, p)
			}
			row["peers"] = peers
			rows = append(rows, row)
		}
		data := map[string]any{"interfaces": rows}
		for key, value := range fields {
			data[key] = value
		}
		out["wireguard"] = c.section(data, partial)
	}
	return out
}

func parseRoutes(raw json.RawMessage) ([]map[string]any, error) {
	if len(raw) > 0 && raw[0] == '{' {
		var wrapper map[string]json.RawMessage
		if json.Unmarshal(raw, &wrapper) != nil {
			return nil, errProbe
		}
		if len(wrapper) == 0 {
			return []map[string]any{}, nil
		}
		var ok bool
		raw, ok = wrapper["route6"]
		if !ok {
			return nil, errProbe
		}
	}
	var routes []struct {
		Destination string  `json:"destination"`
		Gateway     string  `json:"gateway"`
		Interface   string  `json:"interface"`
		Metric      *uint64 `json:"metric"`
		Rejecting   bool    `json:"rejecting"`
	}
	if json.Unmarshal(raw, &routes) != nil || routes == nil {
		return nil, errProbe
	}
	out := []map[string]any{}
	for _, r := range routes {
		if _, err := netip.ParsePrefix(r.Destination); err != nil {
			return nil, errProbe
		}
		v := map[string]any{"dst": r.Destination, "dev": r.Interface, "table": "unknown"}
		if r.Gateway != "" {
			gateway, e := netip.ParseAddr(r.Gateway)
			if e != nil {
				return nil, errProbe
			}
			v["gateway"] = gateway.String()
		}
		if r.Metric != nil {
			v["metric"] = *r.Metric
		}
		if r.Rejecting {
			v["active"] = false
		}
		out = append(out, v)
	}
	return out, nil
}
