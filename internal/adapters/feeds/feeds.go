package feeds

import (
	"ainfra/internal/domain"
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ulikunitz/xz"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

const DebianURL = "https://security-tracker.debian.org/tracker/data/json"
const UbuntuURL = "https://security-metadata.canonical.com/osv/osv-all.tar.xz"

func Debian(r io.Reader) ([]domain.Advisory, error) {
	d := json.NewDecoder(io.LimitReader(r, 256<<20))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return nil, errors.New("invalid Debian snapshot")
	}
	out := []domain.Advisory{}
	for d.More() {
		tok, e = d.Token()
		if e != nil {
			return nil, e
		}
		source, ok := tok.(string)
		if !ok {
			return nil, errors.New("invalid source")
		}
		var issues map[string]struct {
			Description string `json:"description"`
			Releases    map[string]struct {
				Status  string `json:"status"`
				Fixed   string `json:"fixed_version"`
				Urgency string `json:"urgency"`
				NoDSA   string `json:"nodsa"`
				Reason  string `json:"nodsa_reason"`
			} `json:"releases"`
		}
		if e = d.Decode(&issues); e != nil {
			return nil, e
		}
		for id, v := range issues {
			for release, status := range v.Releases {
				if status.Status == "" {
					return nil, errors.New("missing release status")
				}
				out = append(out, domain.Advisory{ID: id, NativeID: id, Provider: "debian", Distro: "debian", Release: release, Source: source, Fixed: status.Fixed, Status: status.Status, Severity: status.Urgency, Notes: status.NoDSA + " " + status.Reason, URL: "https://security-tracker.debian.org/tracker/" + id})
			}
		}
	}
	if _, e = d.Token(); e != nil {
		return nil, e
	}
	if d.Decode(new(any)) != io.EOF || len(out) == 0 {
		return nil, errors.New("empty or trailing Debian snapshot")
	}
	return out, nil
}

type osv struct {
	ID        string   `json:"id"`
	Upstream  []string `json:"upstream"`
	Aliases   []string `json:"aliases"`
	Withdrawn string   `json:"withdrawn"`
	Affected  []struct {
		Package struct {
			Name      string `json:"name"`
			Ecosystem string `json:"ecosystem"`
		} `json:"package"`
		Ranges []struct {
			Type   string              `json:"type"`
			Events []map[string]string `json:"events"`
		} `json:"ranges"`
	} `json:"affected"`
}

func Ubuntu(b []byte) ([]domain.Advisory, error) {
	var v osv
	if e := json.Unmarshal(b, &v); e != nil {
		return nil, e
	}
	if v.ID == "" {
		return nil, errors.New("missing OSV identity")
	}
	ids := []string{}
	for _, id := range append(v.Upstream, v.Aliases...) {
		if strings.HasPrefix(id, "CVE-") {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		if strings.HasPrefix(v.ID, "UBUNTU-CVE-") {
			ids = []string{strings.TrimPrefix(v.ID, "UBUNTU-")}
		} else {
			ids = []string{v.ID}
		}
	}
	out := []domain.Advisory{}
	for _, p := range v.Affected {
		eco := p.Package.Ecosystem
		if !strings.HasPrefix(eco, "Ubuntu:") {
			continue
		}
		eco = strings.TrimPrefix(eco, "Ubuntu:")
		channel := ""
		if strings.HasPrefix(eco, "Pro:") {
			channel = "Pro"
			eco = strings.TrimPrefix(eco, "Pro:")
		}
		if len(eco) < 5 || eco[2] != '.' {
			continue
		}
		for _, rr := range p.Ranges {
			if rr.Type != "ECOSYSTEM" {
				continue
			}
			introduced := "0"
			fixed := ""
			for _, ev := range rr.Events {
				if v, ok := ev["introduced"]; ok {
					introduced = v
				}
				if v, ok := ev["fixed"]; ok {
					fixed = v
				}
			}
			for _, id := range ids {
				out = append(out, domain.Advisory{ID: id, NativeID: v.ID, Provider: "ubuntu", Distro: "ubuntu", Release: eco, Channel: channel, Source: p.Package.Name, Introduced: introduced, Fixed: fixed, Status: "unknown", Withdrawn: v.Withdrawn != "", URL: "https://ubuntu.com/security/" + id})
			}
		}
	}
	return out, nil
}
func UbuntuArchive(r io.Reader) ([]domain.Advisory, error) {
	xr, e := xz.NewReader(io.LimitReader(r, 80<<20))
	if e != nil {
		return nil, e
	}
	limited := &io.LimitedReader{R: xr, N: 1 << 30}
	tr := tar.NewReader(limited)
	byKey := map[string]domain.Advisory{}
	count := 0
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		clean := path.Clean(h.Name)
		if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return nil, errors.New("unsafe archive path")
		}
		if !strings.HasSuffix(clean, ".json") {
			continue
		}
		if h.Size > 4<<20 {
			return nil, errors.New("OSV record too large")
		}
		b, e := io.ReadAll(tr)
		if e != nil {
			return nil, e
		}
		ads, e := Ubuntu(b)
		if e != nil {
			return nil, e
		}
		count++
		if count > 200000 {
			return nil, errors.New("too many records")
		}
		for _, a := range ads {
			k := a.ID + "/" + a.Release + "/" + a.Channel + "/" + a.Source
			old, exists := byKey[k]
			if !exists || strings.Contains(a.NativeID, "CVE-") || !strings.Contains(old.NativeID, "CVE-") {
				byKey[k] = a
			}
		}
	}
	if limited.N <= 0 || len(byKey) == 0 {
		return nil, errors.New("empty or oversized OSV archive")
	}
	out := make([]domain.Advisory, 0, len(byKey))
	for _, a := range byKey {
		out = append(out, a)
	}
	return out, nil
}

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}
type Source struct {
	ID   string
	URL  string
	HTTP HTTPClient
}
type Snapshot struct {
	Records        []domain.Advisory
	ETag, Modified string
	Unchanged      bool
}

func (s Source) Fetch(ctx context.Context, previous domain.Feed) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "GET", s.URL, nil)
	if e != nil {
		return Snapshot{}, e
	}
	if previous.ETag != "" {
		req.Header.Set("If-None-Match", previous.ETag)
	}
	if previous.SourceModified != "" {
		req.Header.Set("If-Modified-Since", previous.SourceModified)
	}
	resp, e := s.HTTP.Do(req)
	if e != nil {
		return Snapshot{}, errors.New("feed connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 304 {
		return Snapshot{Unchanged: true}, nil
	}
	if resp.StatusCode != 200 {
		return Snapshot{}, fmt.Errorf("feed HTTP %d", resp.StatusCode)
	}
	var ads []domain.Advisory
	switch s.ID {
	case "debian":
		ads, e = Debian(resp.Body)
	case "ubuntu":
		ads, e = UbuntuArchive(resp.Body)
	default:
		e = errors.New("unknown provider")
	}
	if e != nil {
		return Snapshot{}, e
	}
	return Snapshot{Records: ads, ETag: resp.Header.Get("ETag"), Modified: resp.Header.Get("Last-Modified")}, nil
}
