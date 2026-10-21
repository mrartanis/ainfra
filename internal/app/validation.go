package app

import (
	"ainfra/internal/domain"
	"bytes"
	"encoding/json"
	"strings"
)

func validateObservation(name string, b json.RawMessage) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || len(b) > 16<<20 {
		return ErrInvalid
	}
	switch name {
	case "os":
		var v domain.OSData
		if b[0] != '{' || strict(b, &v) != nil {
			return ErrInvalid
		}
	case "packages":
		var v domain.PackageData
		if b[0] != '{' || strict(b, &v) != nil || len(v.Packages) > 100000 {
			return ErrInvalid
		}
	case "checks", "disks":
		if b[0] != '[' {
			return ErrInvalid
		}
	default:
		if b[0] != '{' {
			return ErrInvalid
		}
	}
	var value any
	if json.Unmarshal(b, &value) != nil {
		return ErrInvalid
	}
	return validateJSONValue(value, 0)
}
func validateJSONValue(v any, depth int) error {
	if depth > 16 {
		return ErrInvalid
	}
	switch x := v.(type) {
	case map[string]any:
		if len(x) > 256 {
			return ErrInvalid
		}
		for key, value := range x {
			if len(key) > 256 {
				return ErrInvalid
			}
			normalized := strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(key))
			if contains([]string{"password", "privatekey", "presharedkey", "token", "secret", "authorization", "credentials"}, normalized) {
				return ErrInvalid
			}
			if err := validateJSONValue(value, depth+1); err != nil {
				return err
			}
		}
	case []any:
		if len(x) > 100000 {
			return ErrInvalid
		}
		for _, value := range x {
			if err := validateJSONValue(value, depth+1); err != nil {
				return err
			}
		}
	case string:
		if len(x) > 1<<20 {
			return ErrInvalid
		}
	}
	return nil
}
