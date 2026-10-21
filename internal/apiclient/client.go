package apiclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	Base  string
	Token string
	HTTP  *http.Client
}

func HTTP(caFile string) (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		b, e := os.ReadFile(caFile)
		if e != nil {
			return nil, e
		}
		pool, e := x509.SystemCertPool()
		if e != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(b) {
			return nil, fmt.Errorf("invalid CA")
		}
		tr.TLSClientConfig.RootCAs = pool
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}, nil
}
func ValidateURL(raw string, loopback bool) error {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid endpoint")
	}
	if u.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(u.Hostname())
	if loopback && u.Scheme == "http" && ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("HTTPS required (HTTP allowed only for literal loopback)")
}
func ReadSecret(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return "", e
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 4096 {
		return "", fmt.Errorf("secret file must be regular, <=4096 bytes and mode 0600")
	}
	b, e := io.ReadAll(f)
	return strings.TrimSpace(string(b)), e
}
func (c *Client) Do(ctx context.Context, method, path string, in, out any, revision int, page ...*Page) error {
	b := []byte(nil)
	var e error
	if in != nil {
		b, e = json.Marshal(in)
		if e != nil {
			return e
		}
	}
	r, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Base, "/")+"/api/v1/"+path, bytes.NewReader(b))
	if e != nil {
		return e
	}
	r.Header.Set("Authorization", "Bearer "+c.Token)
	r.Header.Set("Content-Type", "application/json")
	if revision > 0 {
		r.Header.Set("If-Match", fmt.Sprint(revision))
	}
	resp, e := c.HTTP.Do(r)
	if e != nil {
		return fmt.Errorf("API connection failed")
	}
	defer resp.Body.Close()
	if len(page) > 0 && page[0] != nil {
		p := page[0]
		p.NextOffset = -1
		p.Total, _ = strconv.Atoi(resp.Header.Get("X-Total-Count"))
		p.Limit, _ = strconv.Atoi(resp.Header.Get("X-Page-Limit"))
		p.Offset, _ = strconv.Atoi(resp.Header.Get("X-Page-Offset"))
		if raw := resp.Header.Get("X-Next-Offset"); raw != "" {
			p.NextOffset, _ = strconv.Atoi(raw)
		}
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if e != nil {
		return fmt.Errorf("API read failed")
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("API status %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if s, ok := out.(*string); ok {
		*s = string(data)
		return nil
	}
	return json.Unmarshal(data, out)
}

type Page struct {
	Total      int `json:"total"`
	Limit      int `json:"limit"`
	Offset     int `json:"offset"`
	NextOffset int `json:"next_offset"`
}

func (c *Client) ForEach(ctx context.Context, path string, visit func(json.RawMessage) error) error {
	u, e := url.Parse(path)
	if e != nil {
		return e
	}
	offset := 0
	for {
		q := u.Query()
		q.Set("offset", strconv.Itoa(offset))
		q.Set("limit", "100")
		u.RawQuery = q.Encode()
		var rows []json.RawMessage
		var page Page
		if e = c.Do(ctx, "GET", u.String(), nil, &rows, 0, &page); e != nil {
			return e
		}
		for _, row := range rows {
			if e = visit(row); e != nil {
				return e
			}
		}
		if page.NextOffset < 0 {
			return nil
		}
		if page.NextOffset <= offset {
			return fmt.Errorf("API returned invalid pagination")
		}
		offset = page.NextOffset
	}
}
