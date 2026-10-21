package linuxcollect_test

import (
	"ainfra/internal/adapters/linuxcollect"
	"os"
	"reflect"
	"testing"
)

func FuzzAPTParsers(f *testing.F) {
	for _, path := range []string{"../../../fixtures/linux/policy.txt", "../../../fixtures/linux/show.txt"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(string(raw))
	}
	f.Add("Installed:\nCandidate:\n")
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 64<<10 {
			t.Skip()
		}
		a, ea := linuxcollect.ParsePolicy(raw)
		b, eb := linuxcollect.ParsePolicy(raw)
		if (ea == nil) != (eb == nil) || !reflect.DeepEqual(a, b) {
			t.Fatal("policy parsing is nondeterministic")
		}
		if !reflect.DeepEqual(linuxcollect.ParseOrigins(raw), linuxcollect.ParseOrigins(raw)) || !reflect.DeepEqual(linuxcollect.ParseShow(raw), linuxcollect.ParseShow(raw)) {
			t.Fatal("APT metadata parsing is nondeterministic")
		}
	})
}
