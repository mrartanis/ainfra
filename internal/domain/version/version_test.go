package version_test

import (
	"ainfra/internal/domain/version"
	"os/exec"
	"testing"
)

func TestDebianOrderingAgainstDpkg(t *testing.T) {
	versions := []string{"0", "1", "1.0", "1.0~rc1", "1.0-1", "1.0-1+b1", "1.0-1+deb12u1", "1.0-1~bpo12+1", "1:0.1", "2:0", "1.01", "1.0001", "1.0+git2024", "1.0-0", "1.0~~", "1.0a", "1.0+"}
	for _, a := range versions {
		for _, b := range versions {
			got, e := version.Compare(a, b)
			if e != nil {
				t.Fatal(e)
			}
			want := 0
			if exec.Command("dpkg", "--compare-versions", a, "lt", b).Run() == nil {
				want = -1
			} else if exec.Command("dpkg", "--compare-versions", a, "gt", b).Run() == nil {
				want = 1
			}
			if got != want {
				t.Fatalf("%s vs %s: %d want %d", a, b, got, want)
			}
		}
	}
}
func TestInvalid(t *testing.T) {
	for _, s := range []string{"", "abc", "1:bad", "1 2", "-1", "1/2"} {
		if _, e := version.Compare(s, "1"); e == nil {
			t.Fatal(s)
		}
	}
}
func FuzzCompare(f *testing.F) {
	f.Add("1.0~rc1", "1.0")
	f.Fuzz(func(t *testing.T, a, b string) {
		x, e := version.Compare(a, b)
		if e != nil {
			return
		}
		y, e := version.Compare(b, a)
		if e != nil || x != -y {
			t.Fatal("antisymmetry")
		}
	})
}
