package feeds_test

import (
	"ainfra/internal/adapters/feeds"
	"ainfra/internal/app"
	"ainfra/internal/domain"
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

func TestUbuntuMultipleIntervalsDoNotLoseEarlierAffectedVersions(t *testing.T) {
	ads, err := feeds.Ubuntu([]byte(`{"id":"UBUNTU-CVE-2026-1","affected":[{"package":{"name":"foo","ecosystem":"Ubuntu:24.04:LTS"},"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"1"},{"fixed":"2"},{"introduced":"3"},{"fixed":"4"}]}]}]}`))
	if err != nil || len(ads) != 1 {
		t.Fatal(ads, err)
	}
	for _, test := range []struct{ version, assessment string }{{"1.5", "affected"}, {"2.5", "not_affected"}, {"3.5", "affected"}, {"4", "not_affected"}} {
		p := domain.Package{Source: "foo", SourceVersion: test.version, Version: test.version, Origin: "Ubuntu", OriginEvidence: "apt-policy", Suite: "noble", CandidateOrigin: "Ubuntu", CandidateSuite: "noble", CandidateOriginEvidence: "apt-policy", CandidateSource: "foo", CandidateSourceVersion: "4"}
		assessment, remediation := app.Assess(p, ads[0])
		if assessment != test.assessment || (assessment == "affected" && remediation != "candidate_fixes") {
			t.Fatal(test, assessment, remediation)
		}
	}
}

func TestUbuntuRangeBoundariesAndCandidateReintroduction(t *testing.T) {
	for _, test := range []struct{ events, installed, candidate, want, rem string }{
		{`{"introduced":"0"},{"last_affected":"2"}`, "2", "3", "affected_or_under_investigation", "unknown"},
		{`{"introduced":"0"},{"last_affected":"2"}`, "2.1", "3", "not_affected", "unknown"},
		{`{"introduced":"0"},{"limit":"2"}`, "2", "3", "not_affected", "unknown"},
		{`{"introduced":"0"},{"fixed":"2"},{"introduced":"3"},{"fixed":"4"}`, "1", "3.5", "affected", "fix_published_not_candidate"},
		{`{"fixed":"4"},{"introduced":"3"},{"fixed":"2"},{"introduced":"0"}`, "1", "2.5", "affected", "candidate_fixes"},
	} {
		payload := `{"id":"UBUNTU-CVE-2026-1","affected":[{"package":{"name":"foo","ecosystem":"Ubuntu:24.04:LTS"},"ranges":[{"type":"ECOSYSTEM","events":[` + test.events + `]}]}]}`
		ads, err := feeds.Ubuntu([]byte(payload))
		if err != nil || len(ads) != 1 {
			t.Fatal(ads, err)
		}
		p := domain.Package{Source: "foo", SourceVersion: test.installed, Version: test.installed, Origin: "Ubuntu", OriginEvidence: "apt-policy", Suite: "noble", CandidateOrigin: "Ubuntu", CandidateSuite: "noble", CandidateOriginEvidence: "apt-policy", CandidateSource: "foo", CandidateSourceVersion: test.candidate}
		as, rem := app.Assess(p, ads[0])
		if as != test.want || rem != test.rem {
			t.Fatal(test, as, rem)
		}
	}
}
