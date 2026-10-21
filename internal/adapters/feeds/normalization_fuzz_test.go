package feeds_test

import (
	"ainfra/internal/adapters/feeds"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"bytes"
	"os"
	"testing"
)

func FuzzFeedNormalization(f *testing.F) {
	for _, path := range []string{"../../../fixtures/feeds/debian-openssl.json", "../../../fixtures/feeds/ubuntu-cve.json"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(raw)
	}
	f.Add([]byte(`{"id":"USN-fixture","affected":[]}`))
	f.Add([]byte(`{"openssl":{"CVE-fixture":{"releases":{"bookworm":{"status":"open"}}}}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 64<<10 {
			t.Skip()
		}
		debian, err := feeds.Debian(bytes.NewReader(raw))
		if err == nil {
			for _, a := range debian {
				if a.Provider != "debian" || a.Distro != "debian" {
					t.Fatal("provider identity controlled by payload")
				}
			}
		}
		ubuntu, err := feeds.Ubuntu(raw)
		if err == nil {
			for _, a := range ubuntu {
				if a.Provider != "ubuntu" || a.Distro != "ubuntu" {
					t.Fatal("provider identity controlled by payload")
				}
				if a.Status == "notice_only" {
					assessment, remediation := app.Assess(domain.Package{Source: a.Source, SourceVersion: "1", Origin: "Ubuntu", OriginEvidence: "fixture"}, a)
					if assessment != "unknown" || remediation != "unknown" {
						t.Fatal("notice-only record became a verified verdict")
					}
				}
			}
		}
	})
}
