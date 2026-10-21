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
	out := []domain.Advisory{}
	err := DebianStream(r, func(a domain.Advisory) error { out = append(out, a); return nil })
	return out, err
}
func DebianStream(r io.Reader, emit func(domain.Advisory) error) error {
	d := json.NewDecoder(io.LimitReader(r, 256<<20))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return errors.New("invalid Debian snapshot")
	}
	count := 0
	for d.More() {
		tok, e = d.Token()
		if e != nil {
			return e
		}
		source, ok := tok.(string)
		if !ok {
			return errors.New("invalid source")
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
			return e
		}
		for id, v := range issues {
			for release, status := range v.Releases {
				if status.Status == "" {
					return errors.New("missing release status")
				}
				if e := emit(domain.Advisory{ID: id, NativeID: id, Provider: "debian", Distro: "debian", Release: release, Source: source, Fixed: status.Fixed, Status: status.Status, Severity: status.Urgency, Notes: status.NoDSA + " " + status.Reason, URL: "https://security-tracker.debian.org/tracker/" + id}); e != nil {
					return e
				}
				count++
			}
		}
	}
	if _, e = d.Token(); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF || count == 0 {
		return errors.New("empty or trailing Debian snapshot")
	}
	return nil
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
			Type   string                `json:"type"`
			Events []domain.VersionEvent `json:"events"`
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
		id = strings.TrimPrefix(id, "UBUNTU-")
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
	status := "unknown"
	if strings.HasPrefix(v.ID, "USN-") {
		status = "notice_only"
	}
	if strings.HasPrefix(v.ID, "LSN-") {
		return out, nil
	}
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

		ranges := []domain.AffectedRange{}
		for _, rr := range p.Ranges {
			if rr.Type != "ECOSYSTEM" {
				continue
			}
			if len(rr.Events) == 0 {
				return nil, errors.New("empty OSV events")
			}
			hasIntroduction := false
			for _, event := range rr.Events {
				hasIntroduction = hasIntroduction || event.Introduced != ""
				fields := 0
				for _, v := range []string{event.Introduced, event.Fixed, event.LastAffected, event.Limit} {
					if v != "" {
						fields++
					}
				}
				if fields != 1 {
					return nil, errors.New("invalid OSV event")
				}
			}
			if !hasIntroduction {
				return nil, errors.New("OSV range missing introduction")
			}
			ranges = append(ranges, domain.AffectedRange{Events: rr.Events})
		}
		if len(ranges) == 0 {
			continue
		}
		introduced, fixed := "", ""
		complex := len(ranges) > 1
		if len(ranges) == 1 {
			starts, ends := 0, 0
			for _, event := range ranges[0].Events {
				if event.Introduced != "" {
					introduced = event.Introduced
					starts++
				}
				if event.Fixed != "" {
					fixed = event.Fixed
					ends++
				}
				if event.LastAffected != "" || event.Limit != "" {
					complex = true
				}
			}
			complex = complex || starts != 1 || ends > 1
		}
		for _, id := range ids {
			advisory := domain.Advisory{ID: id, NativeID: v.ID, Provider: "ubuntu", Distro: "ubuntu", Release: eco, Channel: channel, Source: p.Package.Name, Introduced: introduced, Fixed: fixed, Status: status, Withdrawn: v.Withdrawn != "", URL: "https://ubuntu.com/security/" + id}
			if complex {
				advisory.Ranges = ranges
				advisory.Introduced = ""
				advisory.Fixed = ""
			}
			out = append(out, advisory)
		}

	}
	return out, nil
}
func UbuntuArchive(r io.Reader) ([]domain.Advisory, error) {
	stage, e := NewStage("")
	if e != nil {
		return nil, e
	}
	defer stage.Close()
	if e = UbuntuArchiveStream(r, stage.Emit); e != nil {
		return nil, e
	}
	out := []domain.Advisory{}
	e = stage.Walk(func(_ string, ads []domain.Advisory) error { out = append(out, ads...); return nil })
	return out, e
}
func UbuntuArchiveStream(r io.Reader, emit func(domain.Advisory) error) error {
	xr, e := xz.NewReader(io.LimitReader(r, 80<<20))
	if e != nil {
		return e
	}
	limited := &io.LimitedReader{R: xr, N: 16 << 30}
	tr := tar.NewReader(limited)
	emitted := 0
	count := 0
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		clean := path.Clean(h.Name)
		if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return errors.New("unsafe archive path")
		}
		if !strings.HasSuffix(clean, ".json") {
			continue
		}
		if h.Size > 16<<20 {
			return errors.New("OSV record too large")
		}
		b, e := io.ReadAll(tr)
		if e != nil {
			return e
		}
		ads, e := Ubuntu(b)
		if e != nil {
			return e
		}
		count++
		if count > 200000 {
			return errors.New("too many records")
		}
		for _, a := range ads {
			if a.Release != "22.04:LTS" && a.Release != "24.04:LTS" && a.Release != "26.04:LTS" {
				continue
			}
			if e := emit(a); e != nil {
				return e
			}
			emitted++
		}
	}
	if limited.N <= 0 || emitted == 0 {
		return errors.New("empty or oversized OSV archive")
	}
	return nil
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
	out := []domain.Advisory{}
	snapshot, err := s.FetchTo(ctx, previous, func(a domain.Advisory) error { out = append(out, a); return nil })
	snapshot.Records = out
	return snapshot, err
}
func (s Source) FetchTo(ctx context.Context, previous domain.Feed, emit func(domain.Advisory) error) (Snapshot, error) {
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
	switch s.ID {
	case "debian":
		e = DebianStream(resp.Body, emit)
	case "ubuntu":
		e = UbuntuArchiveStream(resp.Body, emit)
	default:
		e = errors.New("unknown provider")
	}
	if e != nil {
		return Snapshot{}, e
	}
	return Snapshot{ETag: resp.Header.Get("ETag"), Modified: resp.Header.Get("Last-Modified")}, nil
}
