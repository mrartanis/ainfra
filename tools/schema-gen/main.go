// Generates the published wire schemas from JSON field names, plus explicit
// transport invariants. Run: go run ./tools/schema-gen
package main

import (
	"ainfra/internal/domain"
	"encoding/json"
	"os"
	"reflect"
	"strings"
)

type schema map[string]any

var defs = map[string]any{}

func shape(t reflect.Type) any {
	if t == reflect.TypeOf(json.RawMessage{}) {
		return schema{}
	}
	if t.PkgPath() == "time" && t.Name() == "Time" {
		return schema{"type": "string", "format": "date-time"}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return schema{"anyOf": []any{shape(t.Elem()), schema{"type": "null"}}}
	case reflect.String:
		return schema{"type": "string"}
	case reflect.Bool:
		return schema{"type": "boolean"}
	case reflect.Int, reflect.Int64, reflect.Uint64:
		return schema{"type": "integer"}
	case reflect.Float64:
		return schema{"type": "number"}
	case reflect.Interface:
		return schema{}
	case reflect.Map:
		return schema{"type": []string{"object", "null"}, "additionalProperties": shape(t.Elem())}
	case reflect.Slice:
		return schema{"type": []string{"array", "null"}, "items": shape(t.Elem())}
	case reflect.Struct:
		name := t.Name()
		if _, ok := defs[name]; !ok {
			defs[name] = schema{}
			props := schema{}
			required := []string{}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				tag := f.Tag.Get("json")
				parts := strings.Split(tag, ",")
				if parts[0] == "-" {
					continue
				}
				props[parts[0]] = shape(f.Type)
				if !strings.Contains(tag, "omitempty") {
					required = append(required, parts[0])
				}
			}
			defs[name] = schema{"type": "object", "properties": props, "required": required, "additionalProperties": false}
		}
		return schema{"$ref": "#/$defs/" + name}
	default:
		panic(t.String())
	}
}
func write(path string, v any) {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		panic(e)
	}
	if e = os.WriteFile(path, append(b, '\n'), 0644); e != nil {
		panic(e)
	}
}
func main() {
	for _, v := range []any{domain.Report{}, domain.Ack{}, domain.Node{}, domain.Document{}, domain.Principal{}, domain.Token{}, domain.Relation{}, domain.PackageData{}, domain.Finding{}, domain.Feed{}, domain.Attention{}, domain.Event{}} {
		shape(reflect.TypeOf(v))
	}
	report := defs["Report"].(schema)
	props := report["properties"].(schema)
	props["schema_version"] = schema{"const": 1}
	props["sequence"] = schema{"type": "integer", "minimum": 1}
	for _, name := range []string{"report_id", "node_id", "agent_instance_id"} {
		props[name] = schema{"type": "string", "minLength": 1, "maxLength": 100}
	}
	props["collectors"] = schema{"type": "object", "maxProperties": 16, "propertyNames": schema{"enum": []string{"os", "packages", "network", "wireguard", "disks", "checks"}}, "additionalProperties": schema{"$ref": "#/$defs/Section"}}
	defs["Section"].(schema)["properties"].(schema)["status"] = schema{"enum": []string{"ok", "partial", "error", "unsupported", "disabled"}}
	for name, source := range map[string]string{"NodeInput": "Node", "DocumentInput": "Document"} {
		full := defs[source].(schema)["properties"].(schema)
		fields := schema{}
		for k, v := range full {
			fields[k] = v
		}
		for _, k := range []string{"observations", "collector_status", "sequence", "last_report", "received_at", "agent_instance_id", "author", "updated_at", "status", "approved_revision", "approved_hash", "approver", "evidence_hash", "needs_revalidation"} {
			if field, ok := fields[k].(schema); ok {
				copy := schema{}
				for name, value := range field {
					copy[name] = value
				}
				copy["readOnly"] = true
				fields[k] = copy
			}
		}
		required := []string{"name"}
		if source == "Document" {
			required = []string{"title", "kind", "body"}
		}
		defs[name] = schema{"type": "object", "properties": fields, "required": required, "additionalProperties": false}
	}
	defs["PrincipalInput"] = schema{"type": "object", "required": []string{"role"}, "properties": schema{"role": schema{"enum": []string{"reader", "editor", "agent", "probe", "admin"}}, "nodes": schema{"type": []string{"array", "null"}, "items": schema{"type": "string"}}, "instance": schema{"type": "string"}}, "additionalProperties": false}
	defs["RelationInput"] = schema{"type": "object", "required": []string{"source", "target", "type"}, "properties": defs["Relation"].(schema)["properties"], "additionalProperties": false}
	root := schema{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "https://infra.invalid/models.schema.json", "$defs": defs}
	write("api/models.schema.json", root)
	write("api/report.schema.json", schema{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "https://infra.invalid/report.schema.json", "$ref": "#/$defs/Report", "$defs": defs})
	paths := schema{}
	type route struct {
		path, method, input, output string
		array                       bool
	}
	routes := []route{
		{"ingest/reports", "post", "Report", "Ack", false}, {"nodes", "get", "", "Node", true}, {"nodes", "post", "NodeInput", "Node", false}, {"nodes/{id}", "get", "", "Node", false}, {"nodes/{id}", "patch", "NodeInput", "Node", false}, {"nodes/{id}/packages", "get", "", "PackageData", false}, {"nodes/{id}/findings", "get", "", "Finding", true}, {"nodes/{id}/context", "get", "", "markdown", false}, {"nodes/{id}/pay", "post", "", "Node", false}, {"nodes/{id}/reenroll", "post", "object", "object", false}, {"attention", "get", "", "Attention", true}, {"search", "get", "", "object", true}, {"findings", "get", "", "Finding", true}, {"findings/{id}/suppress", "post", "object", "object", false}, {"feed-status", "get", "", "Feed", true}, {"documents", "get", "", "Document", true}, {"documents", "post", "DocumentInput", "Document", false}, {"documents/{id}", "get", "", "Document", false}, {"documents/{id}", "patch", "DocumentInput", "Document", false}, {"documents/{id}/approve", "post", "", "Document", false}, {"relations", "get", "", "Relation", true}, {"relations", "post", "RelationInput", "Relation", false}, {"relations/{id}", "delete", "", "object", false}, {"events", "get", "", "Event", true}, {"tokens", "get", "", "Token", true}, {"tokens", "post", "PrincipalInput", "object", false}, {"tokens/{id}", "delete", "", "object", false}, {"me", "get", "", "Principal", false}, {"settings", "get", "", "object", false}, {"export", "get", "", "object", false}}
	ref := func(name string) any {
		if name == "object" {
			return schema{"type": "object"}
		}
		return schema{"$ref": "./models.schema.json#/$defs/" + name}
	}
	for _, r := range routes {
		path := "/api/v1/" + r.path
		item, ok := paths[path].(schema)
		if !ok {
			item = schema{}
			paths[path] = item
		}
		response := ref(r.output)
		contentType := "application/json"
		if r.output == "markdown" {
			response = schema{"type": "string"}
			contentType = "text/markdown"
		}
		if r.array {
			response = schema{"type": "array", "items": response}
		}
		status := "200"
		if r.method == "post" && (r.path == "nodes" || r.path == "documents" || r.path == "tokens") {
			status = "201"
		}
		operation := schema{"operationId": strings.NewReplacer("/", "_", "{", "", "}", "").Replace(r.method + "_" + r.path), "responses": schema{status: schema{"description": "Success", "content": schema{contentType: schema{"schema": response}}}, "default": schema{"description": "Error; credentials and raw stderr are never included", "content": schema{"application/json": schema{"schema": schema{"type": "object", "required": []string{"error"}, "properties": schema{"error": schema{"type": "string"}}}}}}}}
		params := []any{}
		if strings.Contains(path, "{id}") {
			params = append(params, schema{"name": "id", "in": "path", "required": true, "schema": schema{"type": "string"}})
		}
		if r.method == "patch" || strings.HasSuffix(r.path, "/approve") || strings.HasSuffix(r.path, "/pay") || strings.HasSuffix(r.path, "/reenroll") {
			params = append(params, schema{"name": "If-Match", "in": "header", "required": true, "schema": schema{"type": "string"}, "description": "Current revision, optionally quoted. Conflict returns 409."})
		}
		if len(params) > 0 {
			operation["parameters"] = params
		}
		if r.input != "" {
			operation["requestBody"] = schema{"required": true, "content": schema{"application/json": schema{"schema": ref(r.input)}}}
		}
		item[r.method] = operation
	}
	write("api/openapi.json", schema{"openapi": "3.1.0", "info": schema{"title": "Infra API", "version": "0.1.0", "description": "HTTP API behind a TLS reverse proxy. Agent/probe credentials are write-only and node-scoped. No cookie authentication or remote execution."}, "security": []any{schema{"bearer": []string{}}}, "components": schema{"securitySchemes": schema{"bearer": schema{"type": "http", "scheme": "bearer"}}}, "paths": paths})
}
