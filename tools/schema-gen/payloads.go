package main

import (
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"reflect"
)

func payloadSchemas() {
	for _, v := range []any{domain.OSData{}, app.Check{}, app.BackupResult{}, app.PaymentResult{}, sqlite.RetentionResult{}} {
		shape(reflect.TypeOf(v))
	}
	// Reports permit omitted optional observation fields. Keep response models
	// separate so their complete marshalled representation remains documented.
	optional := func(source, target string) schema {
		original := defs[source].(schema)["properties"].(schema)
		props := schema{}
		for k, v := range original {
			props[k] = v
		}
		defs[target] = schema{"type": "object", "properties": props, "additionalProperties": false}
		return props
	}
	optional("OSData", "OSObservation")
	optional("Package", "PackageObservation")
	optional("APTRefresh", "APTRefreshObservation")
	pkg := optional("PackageData", "PackageObservationData")
	pkg["packages"] = schema{"type": []string{"array", "null"}, "maxItems": 100000, "items": schema{"$ref": "#/$defs/PackageObservation"}}
	pkg["metadata_refresh"] = schema{"anyOf": []any{schema{"$ref": "#/$defs/APTRefreshObservation"}, schema{"type": "null"}}}
	backup := defs["BackupResult"].(schema)
	backup["required"] = []string{"last_attempt_at", "last_attempt_status"}
	backup["properties"].(schema)["last_attempt_status"] = schema{"enum": []string{"success", "error"}}
	backup["properties"].(schema)["evidence"] = schema{"type": "string", "maxLength": 4096}
	payment := defs["PaymentResult"].(schema)
	payment["required"] = []string{"next_due_date"}
	payment["properties"].(schema)["evidence"] = schema{"type": "string", "maxLength": 4096}
	check := defs["Check"].(schema)
	cp := check["properties"].(schema)
	cp["schema_version"] = schema{"const": 1}
	cp["check_id"] = schema{"type": "string", "minLength": 1, "maxLength": 200}
	cp["kind"] = schema{"enum": []string{"backup", "payment"}}
	cp["status"] = schema{"enum": []string{"ok", "error"}}
	check["allOf"] = []any{
		schema{"if": schema{"properties": schema{"kind": schema{"const": "backup"}}}, "then": schema{"properties": schema{"data": schema{"$ref": "#/$defs/BackupResult"}}}},
		schema{"if": schema{"properties": schema{"kind": schema{"const": "payment"}}}, "then": schema{"properties": schema{"data": schema{"$ref": "#/$defs/PaymentResult"}}}},
	}
	collectors := defs["Report"].(schema)["properties"].(schema)["collectors"].(schema)
	specific := schema{}
	for name, payload := range map[string]any{
		"os":        schema{"$ref": "#/$defs/OSObservation"},
		"packages":  schema{"$ref": "#/$defs/PackageObservationData"},
		"checks":    schema{"type": "array", "maxItems": 1000, "items": schema{"$ref": "#/$defs/Check"}},
		"disks":     schema{"type": "array", "maxItems": 100000},
		"network":   schema{"type": "object", "maxProperties": 256},
		"wireguard": schema{"type": "object", "maxProperties": 256},
	} {
		specific[name] = schema{"allOf": []any{schema{"$ref": "#/$defs/Section"}, schema{"properties": schema{"data": payload}}}}
	}
	collectors["properties"] = specific
	section := defs["Section"].(schema)
	section["properties"].(schema)["error_code"] = schema{"type": "string", "maxLength": 128}
	section["allOf"] = []any{schema{"if": schema{"properties": schema{"status": schema{"enum": []string{"ok", "partial"}}}}, "then": schema{"required": []string{"data"}}}}
	defs["ReenrollInput"] = schema{"type": "object", "required": []string{"agent_instance_id"}, "properties": schema{"agent_instance_id": schema{"type": "string", "minLength": 1}}, "additionalProperties": false}
	defs["SuppressionInput"] = schema{"type": "object", "required": []string{"reason", "until"}, "properties": schema{"reason": schema{"type": "string", "minLength": 1}, "until": schema{"type": "string", "format": "date-time"}}, "additionalProperties": false}
	defs["OK"] = schema{"type": "object", "required": []string{"ok"}, "properties": schema{"ok": schema{"type": "boolean"}}, "additionalProperties": false}
	defs["IssuedToken"] = schema{"type": "object", "required": []string{"secret", "token"}, "properties": schema{"secret": schema{"type": "string", "description": "Returned once when issuing the token; never returned by token list"}, "token": schema{"$ref": "#/$defs/Token"}}, "additionalProperties": false}
	defs["Settings"] = schema{"type": "object", "properties": schema{"retention": schema{"$ref": "#/$defs/RetentionResult"}}, "additionalProperties": false}
}
