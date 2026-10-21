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
