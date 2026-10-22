package domain

import "testing"

func TestHostingInheritanceUnknownAndCycle(t *testing.T) {
	h := NewHostingIndex([]Node{
		{ID: "host", Labels: map[string]string{"provider": " ExampleProvider "}},
		{ID: "vm", Labels: map[string]string{"provider": "outdated"}},
		{ID: "container"}, {ID: "unknown"}, {ID: "retired", Lifecycle: "retired"},
	}, []Relation{{Source: "vm", Target: "host", Type: "runs_on"}, {Source: "container", Target: "vm", Type: "runs_on"}, {Source: "unknown", Target: "host", Type: "tunnel_peer"}})
	if got := h.Providers("container"); len(got) != 1 || got[0] != "ExampleProvider" {
		t.Fatal(got)
	}
	if got := h.Providers("unknown"); len(got) != 1 || got[0] != UnknownProvider {
		t.Fatal(got)
	}
	if got := h.Descendants("host"); len(got) != 3 || !got["container"] || got["unknown"] {
		t.Fatal(got)
	}
	if len(h.Descendants("missing")) != 0 {
		t.Fatal("unknown host matched nodes")
	}
	h.Parents["host"] = []string{"container"}
	h.Children["container"] = []string{"host"}
	if got := h.Providers("vm"); got[0] != UnknownProvider {
		t.Fatal(got)
	}
	if len(h.Descendants("host")) != 3 {
		t.Fatal("cycle traversal")
	}
}
