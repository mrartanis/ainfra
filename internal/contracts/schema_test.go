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
