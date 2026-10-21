package feeds_test

import (
	"ainfra/internal/adapters/feeds"
	"ainfra/internal/domain"
	"os"
	"testing"
)

func TestDiskStageDeduplicatesAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	s, e := feeds.NewStage(dir)
	if e != nil {
		t.Fatal(e)
	}
	a := domain.Advisory{ID: "CVE-1", NativeID: "USN-1", Source: "../../openssl", Release: "24.04:LTS", Fixed: "2"}
	if e = s.Emit(a); e != nil {
		t.Fatal(e)
	}
	a.NativeID = "UBUNTU-CVE-1"
	a.Fixed = "3"
	if e = s.Emit(a); e != nil {
		t.Fatal(e)
	}
	a.NativeID = "USN-2"
	a.Fixed = "4"
	if e = s.Emit(a); e != nil {
		t.Fatal(e)
	}
	calls := 0
	e = s.Walk(func(source string, ads []domain.Advisory) error {
		calls++
		if source != "../../openssl" || len(ads) != 1 || ads[0].Fixed != "3" {
			t.Fatal(source, ads)
		}
		return nil
	})
	if e != nil || calls != 1 {
		t.Fatal(e, calls)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("staging left on disk")
	}
}

func TestStagePreservesSeparateRangesOfSameCanonicalAdvisory(t *testing.T) {
	stage, err := feeds.NewStage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Close()
	a := domain.Advisory{ID: "CVE-1", NativeID: "UBUNTU-CVE-1", Source: "foo", Release: "24.04:LTS", Introduced: "1", Fixed: "2"}
	if err = stage.Emit(a); err != nil {
		t.Fatal(err)
	}
	a.Introduced = "3"
	a.Fixed = "4"
	if err = stage.Emit(a); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err = stage.Emit(a); err != nil {
			t.Fatal(err)
		}
	}
	if err = stage.Walk(func(_ string, ads []domain.Advisory) error {
		if len(ads) != 1 || len(ads[0].Ranges) != 2 {
			t.Fatal("range lost", ads)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
