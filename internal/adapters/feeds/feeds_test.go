package feeds_test

import (
	"ainfra/internal/adapters/feeds"
	"os"
	"strings"
	"testing"
)

func TestRealDebianFixture(t *testing.T) {
	f, e := os.Open("../../../fixtures/feeds/debian-openssl.json")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	ads, e := feeds.Debian(f)
	if e != nil || len(ads) == 0 {
		t.Fatal(len(ads), e)
	}
	for _, a := range ads {
		if a.Source != "openssl" || a.Provider != "debian" {
			t.Fatal(a)
		}
	}
}
func TestEmptyAndMalformedRejected(t *testing.T) {
	for _, s := range []string{"{}", "[]", "null", "broken"} {
		if _, e := feeds.Debian(strings.NewReader(s)); e == nil {
			t.Fatal(s)
		}
	}
}
func TestUbuntuAmbiguityAndPro(t *testing.T) {
	ads, e := feeds.Ubuntu([]byte(`{"id":"UBUNTU-CVE-2026-1","upstream":["CVE-2026-1"],"affected":[{"package":{"name":"foo","ecosystem":"Ubuntu:Pro:24.04:LTS"},"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"0"}]}]}]}`))
	if e != nil || len(ads) != 1 || ads[0].Fixed != "" || ads[0].Channel != "Pro" || ads[0].ID != "CVE-2026-1" {
		t.Fatal(ads, e)
	}
}

func TestUSNUsesUpstreamIdentityWithoutClaimingApplicability(t *testing.T) {
	ads, e := feeds.Ubuntu([]byte(`{"id":"USN-1-1","upstream":["UBUNTU-CVE-2026-1"],"affected":[{"package":{"name":"foo","ecosystem":"Ubuntu:24.04:LTS"},"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"0"},{"fixed":"2"}]}]}]}`))
	if e != nil || len(ads) != 1 || ads[0].ID != "CVE-2026-1" || ads[0].Status != "notice_only" {
		t.Fatal(ads, e)
	}
}
