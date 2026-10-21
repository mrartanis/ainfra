package linuxcollect

import (
	"ainfra/internal/domain"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type bounded struct {
	buffer bytes.Buffer
	limit  int
}

func (b *bounded) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		return 0, errors.New("output_limit")
	}
	return b.buffer.Write(p)
}

type ExecRunner struct{ Timeout time.Duration }

func (r ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C", "PAGER=cat", "DPKG_PAGER=cat")
	out := &bounded{limit: 8 << 20}
	stderr := &bounded{limit: 64 << 10}
	cmd.Stdout = out
	cmd.Stderr = stderr
	e := cmd.Run()
	if e != nil {
		return nil, errors.New("command_failed")
	}
	return out.buffer.Bytes(), nil
}

type Collector struct {
	wireguardSnapshot string
	run               Runner
	read              func(string) ([]byte, error)
	now               func() time.Time
}

func New(r Runner, read func(string) ([]byte, error), now func() time.Time, snapshots ...string) *Collector {
	path := ""
	if len(snapshots) > 0 {
		path = snapshots[0]
	}
	return &Collector{run: r, read: read, now: now, wireguardSnapshot: path}
}
func (c *Collector) section(v any) domain.Section {
	b, _ := json.Marshal(v)
	return domain.Section{Status: "ok", Complete: true, ObservedAt: c.now(), Data: b}
}
func failed(code string) domain.Section { return domain.Section{Status: "error", ErrorCode: code} }
func ParseDpkg(text string) ([]domain.Package, error) {
	out := []domain.Package{}
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 7 {
			return nil, errors.New("dpkg_format")
		}
		if f[5] == "config-files" || f[5] == "not-installed" {
			continue
		}
		for _, v := range f[:6] {
			if v == "" {
				return nil, errors.New("dpkg_missing")
			}
		}
		out = append(out, domain.Package{Name: f[0], Arch: f[1], Version: f[2], Source: f[3], SourceVersion: f[4], State: f[5], Held: f[6] == "hold", BlockedReason: "unknown"})
	}
	return out, nil
}
func OSRelease(b []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, `"`) {
			if unquoted, e := strconv.Unquote(v); e == nil {
				v = unquoted
			}
		} else {
			v = strings.Trim(v, "'")
		}
		out[k] = v
	}
	return out
}
func (c *Collector) OS(ctx context.Context) domain.Section {
	b, e := c.read("/etc/os-release")
	if e != nil {
		return failed("os_release_unavailable")
	}
	m := OSRelease(b)
	d := domain.OSData{ID: m["ID"], Codename: m["VERSION_CODENAME"], Version: m["VERSION_ID"], Arch: runtime.GOARCH, CPU: runtime.NumCPU(), Reboot: "unknown", VirtualizationHint: "unknown"}
	if b, e = c.read("/sys/class/dmi/id/product_name"); e == nil && len(b) < 512 {
		d.Model = strings.TrimSpace(string(b))
	}
	if b, e = c.run.Run(ctx, "systemd-detect-virt"); e == nil {
		hint := strings.TrimSpace(string(b))
		if len(hint) > 0 && len(hint) < 64 && !strings.ContainsAny(hint, " \t\r\n") {
			d.VirtualizationHint = hint
		}
	}
	if b, e = c.read("/proc/sys/kernel/hostname"); e == nil {
		d.Hostname = strings.TrimSpace(string(b))
	}
	if b, e = c.read("/proc/sys/kernel/osrelease"); e == nil {
		d.Kernel = strings.TrimSpace(string(b))
	}
	if b, e = c.read("/proc/meminfo"); e == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "MemFree:") {
				fields := strings.Fields(l)
				if len(fields) > 1 {
					if value, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
						value *= 1024
						d.MemoryFreeBytes = &value
					}
				}
			}
			if strings.HasPrefix(l, "MemTotal:") {
				f := strings.Fields(l)
				if len(f) > 1 {
					v, _ := strconv.ParseUint(f[1], 10, 64)
					d.MemoryBytes = v * 1024
				}
			}
		}
	}
	if b, e = c.read("/proc/uptime"); e == nil {
		f := strings.Fields(string(b))
		if len(f) > 0 {
			if value, err := strconv.ParseFloat(f[0], 64); err == nil && value >= 0 {
				d.UptimeSeconds = &value
			}
		}
	}
	if _, e = c.read("/var/run/reboot-required"); e == nil {
		d.Reboot = "yes"
	}
	return c.section(d)
}
func (c *Collector) Packages(ctx context.Context) domain.Section {
	b, e := c.run.Run(ctx, "dpkg-query", "-W", "-f=${Package}\t${Architecture}\t${Version}\t${source:Package}\t${source:Version}\t${db:Status-Status}\t${db:Status-Want}\n")
	if e != nil {
		return failed("dpkg_unavailable")
	}
	ps, e := ParseDpkg(string(b))
	if e != nil {
		return failed("dpkg_format")
	}
	data := domain.PackageData{Packages: ps, MetadataStatus: "unknown", Simulation: "not_run", MetadataRefresh: c.aptRefresh()}
	holds, he := c.run.Run(ctx, "apt-mark", "showhold")
	held := map[string]bool{}
	for _, v := range strings.Fields(string(holds)) {
		held[v] = true
	}
	partial := he != nil
	// Policy origin evidence is repository metadata, not proof of historical installation origin.
	sources, _ := c.run.Run(ctx, "apt-cache", "policy")
	origins := ParseOrigins(string(sources))
	for start := 0; start < len(ps); start += 64 {
		end := start + 64
		if end > len(ps) {
			end = len(ps)
		}
		args := []string{"policy"}
		for _, p := range ps[start:end] {
			args = append(args, p.Name+":"+p.Arch)
		}
		b, e = c.run.Run(ctx, "apt-cache", args...)
		if e != nil {
			partial = true
			continue
		}
		policies, e := ParsePolicy(string(b))
		if e != nil {
			partial = true
			continue
		}
		showArgs := []string{"show"}
		for i := start; i < end; i++ {
			p := &data.Packages[i]
			p.Held = p.Held || held[p.Name] || held[p.Name+":"+p.Arch]
			pol, ok := policies[p.Name+":"+p.Arch]
			if !ok {
				pol, ok = policies[p.Name]
			}
			if !ok {
				partial = true
				continue
			}
			p.Candidate = pol.Candidate
			if p.Candidate == "(none)" {
				p.Candidate = ""
			}
			if p.Held {
				p.BlockedReason = "held"
			}

			p.Origin, p.Suite, p.OriginEvidence = policyEvidence(pol.InstalledSources, origins)
			p.CandidateOrigin, p.CandidateSuite, p.CandidateOriginEvidence = policyEvidence(pol.CandidateSources, origins)

			if p.Candidate != "" {
				showArgs = append(showArgs, p.Name+":"+p.Arch+"="+p.Candidate)
			}
		}
		if len(showArgs) > 1 {
			b, e = c.run.Run(ctx, "apt-cache", showArgs...)
			if e != nil {
				partial = true
				continue
			}
			metadata := ParseShow(string(b))
			for i := start; i < end; i++ {
				p := &data.Packages[i]
				if m, ok := metadata[p.Name+":"+p.Arch+"="+p.Candidate]; ok {
					p.CandidateSource = m.Source
					p.CandidateSourceVersion = m.SourceVersion
				} else if p.Candidate != "" {
					partial = true
				}
			}
		}
	}
	result := c.section(data)
	if partial {
		result.ErrorCode = "apt_partial" /* installed inventory remains a complete dpkg snapshot; APT uncertainty is in data */
		data.MetadataStatus = "partial"
		result.Data, _ = json.Marshal(data)
	}
	return result
}

type Policy struct {
	CandidateSources []string
	Candidate        string
	InstalledSources []string
}

func ParsePolicy(text string) (map[string]Policy, error) {
	out := map[string]Policy{}
	name := ""
	installed := ""
	current := ""
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if line[0] != ' ' && strings.HasSuffix(line, ":") {
			name = strings.TrimSuffix(line, ":")
			out[name] = Policy{}
			installed = ""
			current = ""
			continue
		}
		if name == "" {
			return nil, errors.New("policy_format")
		}
		p := out[name]
		switch f[0] {
		case "Installed:":
			if len(f) != 2 {
				return nil, errors.New("policy_format")
			}
			installed = f[1]
		case "Candidate:":
			if len(f) != 2 {
				return nil, errors.New("policy_format")
			}
			p.Candidate = f[1]
		case "***":
			if len(f) >= 2 {
				current = f[1]
			}
		default:
			if len(f) == 2 && f[0] != "Version" {
				current = f[0]
			}
			if len(f) >= 3 && (strings.HasPrefix(f[1], "http://") || strings.HasPrefix(f[1], "https://")) {
				if current == installed {
					p.InstalledSources = append(p.InstalledSources, strings.Join(f[1:], " "))
				}
				if current == p.Candidate {
					p.CandidateSources = append(p.CandidateSources, strings.Join(f[1:], " "))
				}
			}
		}
		out[name] = p
	}
	if len(out) == 0 {
		return nil, errors.New("empty_policy")
	}
	return out, nil
}

type Origin struct{ Origin, Suite string }

func ParseOrigins(text string) map[string]Origin {
	out := map[string]Origin{}
	key := ""
	for _, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if len(f) > 2 && (strings.HasPrefix(f[1], "http://") || strings.HasPrefix(f[1], "https://")) {
			key = strings.Join(f[1:], " ")
		}
		l = strings.TrimSpace(l)
		if key != "" && strings.HasPrefix(l, "release ") {
			o := Origin{}
			for _, v := range strings.Split(strings.TrimPrefix(l, "release "), ",") {
				k, val, _ := strings.Cut(v, "=")
				if k == "o" {
					o.Origin = val
				}
				if k == "n" {
					o.Suite = val
				}
				if k == "a" && strings.Contains(val, "backports") {
					o.Suite = val
				}
			}
			out[key] = o
			key = ""
		}
	}
	return out
}
func ParseShow(text string) map[string]domain.Package {
	out := map[string]domain.Package{}
	for _, stanza := range strings.Split(text, "\n\n") {
		m := map[string]string{}
		for _, line := range strings.Split(stanza, "\n") {
			k, v, ok := strings.Cut(line, ": ")
			if ok {
				m[k] = v
			}
		}
		if m["Package"] == "" || m["Version"] == "" || m["Architecture"] == "" {
			continue
		}
		p := domain.Package{Name: m["Package"], Arch: m["Architecture"], Version: m["Version"], Source: m["Package"], SourceVersion: m["Version"]}
		if src := strings.Fields(m["Source"]); len(src) > 0 {
			p.Source = src[0]
			if len(src) > 1 {
				p.SourceVersion = strings.Trim(src[1], "()")
			}
		}
		out[p.Name+":"+p.Arch+"="+p.Version] = p
	}
	return out
}
func (c *Collector) WireGuard(ctx context.Context) domain.Section {
	if c.wireguardSnapshot != "" {
		return c.readWireGuardSnapshot()
	}
	data := map[string]string{}
	for _, field := range []string{"public-key", "peers", "endpoints", "allowed-ips", "latest-handshakes"} {
		b, e := c.run.Run(ctx, "wg", "show", "all", field)
		if e != nil {
			return failed("unavailable_or_permission_denied")
		}
		data[field] = string(b)
	}
	return c.section(data)
}
func (c *Collector) Network(ctx context.Context) domain.Section {
	interfaces, e := net.Interfaces()
	if e != nil {
		return failed("interfaces_unavailable")
	}
	rows := []map[string]any{}
	partial := false
	for _, i := range interfaces {
		addresses, e := i.Addrs()
		if e != nil {
			partial = true
			continue
		}
		as := []string{}
		for _, a := range addresses {
			as = append(as, a.String())
		}
		rows = append(rows, map[string]any{"name": i.Name, "mtu": i.MTU, "flags": i.Flags.String(), "addresses": as})
	}

	data := map[string]any{"interfaces": rows}
	for _, family := range []struct{ flag, key string }{{"-4", "routes"}, {"-6", "routes_ipv6"}} {
		data[family.key+"_status"] = "unknown"
		raw, err := c.run.Run(ctx, "ip", family.flag, "-j", "route", "show", "table", "all")
		var routes []map[string]any
		if err != nil || json.Unmarshal(raw, &routes) != nil || routes == nil {
			partial = true
			continue
		}
		data[family.key] = routes
		data[family.key+"_status"] = "ok"
	}
	result := c.section(data)
	if partial {
		result.Status = "partial"
		result.Complete = false
		result.ErrorCode = "network_incomplete"
	}
	return result
}
func (c *Collector) Collect(ctx context.Context, collectors []string) map[string]domain.Section {
	out := map[string]domain.Section{}
	for _, name := range collectors {
		switch name {
		case "os":
			out[name] = c.OS(ctx)
		case "packages":
			out[name] = c.Packages(ctx)
		case "network":
			out[name] = c.Network(ctx)
		case "disks":
			out[name] = c.Disks(ctx)
		case "wireguard":
			out[name] = c.WireGuard(ctx)
		default:
			out[name] = domain.Section{Status: "unsupported", ErrorCode: fmt.Sprint("collector_unavailable")}
		}
	}
	return out
}

func policyEvidence(sources []string, origins map[string]Origin) (string, string, string) {
	var selected Origin
	for i, source := range sources {
		origin, ok := origins[source]
		if !ok || origin.Origin == "" || origin.Suite == "" {
			return "", "", "unknown"
		}
		if i > 0 && selected != origin {
			return "", "", "ambiguous"
		}
		selected = origin
	}
	if len(sources) == 0 {
		return "", "", ""
	}
	return selected.Origin, selected.Suite, "apt-policy (repository match, historical install origin unproven)"
}
