package linuxcollect_test

import (
	"ainfra/internal/adapters/linuxcollect"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDpkgPreservesSourceAndState(t *testing.T) {
	b, e := os.ReadFile("../../../fixtures/linux/dpkg.tsv")
	if e != nil {
		t.Fatal(e)
	}
	ps, e := linuxcollect.ParseDpkg(string(b))
	if e != nil || len(ps) != 2 {
		t.Fatal(ps, e)
	}
	ps, e = linuxcollect.ParseDpkg("bin\tamd64\t1+b1\tsrc\t1\tinstalled\tinstall\nold\tall\t1\told\t1\tconfig-files\tdeinstall\n")
	if e != nil || len(ps) != 1 || ps[0].SourceVersion != "1" {
		t.Fatal(ps, e)
	}
	if _, e = linuxcollect.ParseDpkg("unexpected output"); e == nil {
		t.Fatal("bad format accepted")
	}
}

type fakeRunner struct{ calls []string }

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return nil, errors.New("denied")
}
func TestWireGuardNeverRequestsKeys(t *testing.T) {
	f := &fakeRunner{}
	c := linuxcollect.New(f, os.ReadFile, time.Now)
	s := c.WireGuard(context.Background())
	if s.Status != "error" {
		t.Fatal(s)
	}
	for _, v := range f.calls {
		for _, bad := range []string{"dump", "showconf", "private-key", "preshared"} {
			if strings.Contains(v, bad) {
				t.Fatal(v)
			}
		}
	}
}
func FuzzDpkg(f *testing.F) {
	f.Add("a\tamd64\t1\ta\t1\tinstalled\tinstall\n")
	f.Fuzz(func(t *testing.T, s string) { linuxcollect.ParseDpkg(s) })
}

func TestCommandOutputLimitCannotBeBypassedByIOCopy(t *testing.T) {
	_, err := (linuxcollect.ExecRunner{}).Run(context.Background(), "/usr/bin/printf", "%8388609s", "")
	if err == nil {
		t.Fatal("collector accepted output larger than 8 MiB")
	}
}

func TestAPTPolicyRetainsCandidateSourcesIndependently(t *testing.T) {
	policies, err := linuxcollect.ParsePolicy("foo:\n  Installed: 1\n  Candidate: 2\n  Version table:\n     2 500\n        500 https://candidate.invalid/ubuntu noble/main amd64 Packages\n *** 1 500\n        500 https://installed.invalid/ubuntu noble/main amd64 Packages\n")
	if err != nil {
		t.Fatal(err)
	}
	p := policies["foo"]
	if len(p.CandidateSources) != 1 || !strings.Contains(p.CandidateSources[0], "candidate.invalid") || len(p.InstalledSources) != 1 || !strings.Contains(p.InstalledSources[0], "installed.invalid") {
		t.Fatal(p)
	}
}

type routeRunner struct{ failIPv6 bool }

func (r routeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name != "ip" {
		return nil, errors.New("unexpected command")
	}
	if strings.Contains(strings.Join(args, " "), "-6") {
		if r.failIPv6 {
			return nil, errors.New("unavailable")
		}
		return []byte(`[{"dst":"default","gateway":"2001:db8::1"}]`), nil
	}
	return []byte(`[{"dst":"default","gateway":"192.0.2.1"}]`), nil
}
func TestNetworkCollectsBothRouteFamiliesAndExposesPartialFailure(t *testing.T) {
	for _, failure := range []bool{false, true} {
		c := linuxcollect.New(routeRunner{failure}, os.ReadFile, time.Now)
		section := c.Network(context.Background())
		if !failure && (!strings.Contains(string(section.Data), "2001:db8::1") || section.Status != "ok") {
			t.Fatal(section)
		}
		if failure && (section.Status != "partial" || section.Complete || !strings.Contains(string(section.Data), "192.0.2.1")) {
			t.Fatal("missing family hidden or good data lost", section)
		}
	}
}

type virtualizationRunner struct{}

func (virtualizationRunner) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	if name == "systemd-detect-virt" {
		return []byte("kvm\n"), nil
	}
	return nil, errors.New("unsupported")
}
func TestOSPreservesModelAndVirtualizationHint(t *testing.T) {
	read := func(path string) ([]byte, error) {
		switch path {
		case "/etc/os-release":
			return []byte("ID=debian\n"), nil
		case "/sys/class/dmi/id/product_name":
			return []byte("Virtual Machine\n"), nil
		default:
			return nil, os.ErrNotExist
		}
	}
	c := linuxcollect.New(virtualizationRunner{}, read, time.Now)
	raw := string(c.OS(context.Background()).Data)
	if !strings.Contains(raw, `"model":"Virtual Machine"`) || !strings.Contains(raw, `"virtualization_hint":"kvm"`) || !strings.Contains(raw, `"uptime_seconds":null`) {
		t.Fatal(raw)
	}
}
