package contracts_test

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/json"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"os"
	"testing"
	"time"
)

func TestReportSchemaAcceptsAgentEnvelopeRejectsUnknownMajor(t *testing.T) {
	raw, e := os.ReadFile("../../api/report.schema.json")
	if e != nil {
		t.Fatal(e)
	}
	var doc any
	if e = json.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if e = compiler.AddResource("https://infra.invalid/report.schema.json", doc); e != nil {
		t.Fatal(e)
	}
	schema, e := compiler.Compile("https://infra.invalid/report.schema.json")
	if e != nil {
		t.Fatal(e)
	}
	report := domain.Report{SchemaVersion: 1, ID: "report-id", NodeID: "node", Instance: "agent", Sequence: 1, ObservedAt: time.Now(), AgentVersion: "0.1.0", Collectors: map[string]domain.Section{"wireguard": {Status: "error", Complete: false, ErrorCode: "permission_denied"}}}
	b, _ := json.Marshal(report)
	var value map[string]any
	json.Unmarshal(b, &value)
	if e = schema.Validate(value); e != nil {
		t.Fatal(e)
	}
	value["schema_version"] = float64(2)
	if e = schema.Validate(value); e == nil {
		t.Fatal("unknown major accepted")
	}
	value["schema_version"] = float64(1)
	value["password"] = "secret"
	if e = schema.Validate(value); e == nil {
		t.Fatal("unexpected field accepted")
	}
}

func TestActualNodeAndDocumentResponsesMatchSchemas(t *testing.T) {
	raw, e := os.ReadFile("../../api/models.schema.json")
	if e != nil {
		t.Fatal(e)
	}
	var models any
	json.Unmarshal(raw, &models)
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if e = compiler.AddResource("https://infra.invalid/models.schema.json", models); e != nil {
		t.Fatal(e)
	}
	svc := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	node, e := svc.SaveNode(context.Background(), admin, domain.Node{Name: "contract"}, 0)
	if e != nil {
		t.Fatal(e)
	}
	doc, e := svc.EditDocument(context.Background(), admin, domain.Document{Title: "note", Kind: "note", Body: "text", Nodes: []string{node.ID}}, 0)
	if e != nil {
		t.Fatal(e)
	}
	for name, value := range map[string]any{"Node": node, "Document": doc} {
		schema, e := compiler.Compile("https://infra.invalid/models.schema.json#/$defs/" + name)
		if e != nil {
			t.Fatal(e)
		}
		b, _ := json.Marshal(value)
		var wire any
		json.Unmarshal(b, &wire)
		if e = schema.Validate(wire); e != nil {
			t.Fatal(name, e)
		}
	}
}

func TestReportSchemaDescribesTypedCollectorPayloads(t *testing.T) {
	raw, err := os.ReadFile("../../api/report.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	json.Unmarshal(raw, &doc)
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err = compiler.AddResource("https://infra.invalid/report.schema.json", doc); err != nil {
		t.Fatal(err)
	}
	validator, err := compiler.Compile("https://infra.invalid/report.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, tc := range []struct {
		name, data string
		valid      bool
	}{
		{"os", `{"id":"ubuntu","uptime_seconds":null,"memory_free_bytes":123}`, true},
		{"os", `{"id":"ubuntu","password":"forbidden"}`, false},
		{"os", `{"cpu_load_percent":"not-a-number"}`, false},
		{"os", `{"memory_bytes":-1}`, false},
		{"packages", `{"packages":[{"name":"openssl","version":"1.0"}],"metadata_status":"unknown"}`, true},
		{"packages", `{"packages":[{"name":"openssl","secret":"forbidden"}]}`, false},
		{"checks", `[{"schema_version":1,"check_id":"copy","kind":"backup","observed_at":"2026-10-21T12:00:00Z","status":"error","data":{"last_attempt_at":"2026-10-21T12:00:00Z","last_attempt_status":"error"}}]`, true},
		{"checks", `[{"schema_version":1,"check_id":"copy","kind":"backup","observed_at":"2026-10-21T12:00:00Z","status":"ok","data":{"last_attempt_at":"2026-10-21T12:00:00Z","last_attempt_status":"invented"}}]`, false},
		{"checks", `[{"schema_version":1,"check_id":"invoice","kind":"payment","observed_at":"2026-10-21T12:00:00Z","status":"error","data":{"evidence":"unavailable"}}]`, true},
		{"checks", `[{"schema_version":1,"check_id":"invoice","kind":"payment","observed_at":"2026-10-21T12:00:00Z","status":"ok","data":{}}]`, false},
	} {
		report := domain.Report{SchemaVersion: 1, ID: "r", NodeID: "node", Instance: "i", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{tc.name: {Status: "ok", Complete: true, ObservedAt: now, Data: json.RawMessage(tc.data)}}}
		raw, _ := json.Marshal(report)
		var wire any
		json.Unmarshal(raw, &wire)
		err = validator.Validate(wire)
		if (err == nil) != tc.valid {
			t.Fatalf("%s valid=%v: %v", tc.name, tc.valid, err)
		}
	}
}
