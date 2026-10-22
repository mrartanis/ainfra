package domain_test

import (
	"ainfra/internal/domain"
	"encoding/json"
	"testing"
)

func TestPackageCountersDistinguishUnknownEmptyAndSummary(t *testing.T) {
	unknown := domain.CountPackages(domain.PackageData{})
	raw, _ := json.Marshal(unknown)
	if unknown.PackageCount != nil || unknown.Updates != nil || unknown.UnknownOrigins != nil {
		t.Fatal(string(raw))
	}
	empty := domain.CountPackages(domain.PackageData{Packages: []domain.Package{}})
	if empty.PackageCount == nil || *empty.PackageCount != 0 {
		t.Fatal("empty inventory must have known zero")
	}
	data := domain.CountPackages(domain.PackageData{Packages: []domain.Package{{Name: "a", Version: "1", Candidate: "2"}}})
	data.Packages = nil
	data = domain.CountPackages(data)
	if data.PackageCount == nil || *data.PackageCount != 1 || data.Updates == nil || *data.Updates != 1 {
		t.Fatal("summary lost computed counters")
	}
	payload, _ := json.Marshal(data)
	node := domain.Node{Observations: map[string]domain.Section{"packages": {Data: payload}}}
	first, _ := json.Marshal(domain.SummarizeNode(node))
	second, _ := json.Marshal(domain.SummarizeNode(domain.SummarizeNode(node)))
	if string(first) != string(second) {
		t.Fatal("summarizing twice changed totals")
	}
}
