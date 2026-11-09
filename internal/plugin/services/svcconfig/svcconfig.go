// Package svcconfig holds parsing helpers shared by the mock service plugins.
package svcconfig

import "fmt"

// scalar renders a YAML/JSON scalar as a string. YAML decodes unquoted numbers
// as int and JSON as float64, so every numeric form is accepted; other types
// yield "".
func scalar(v interface{}) string {
	switch v := v.(type) {
	case string:
		return v
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case uint64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	}
	return ""
}

// String reads a scalar config value as a string (e.g. password: 123456 is
// "123456", not ignored). It returns fallback when the key is absent, empty
// or of another type.
func String(config map[string]interface{}, key, fallback string) string {
	if v := scalar(config[key]); v != "" {
		return v
	}
	return fallback
}

// Port reads a port-like config value as a string; see String.
func Port(config map[string]interface{}, key, fallback string) string {
	return String(config, key, fallback)
}

// Strings reads a list of scalars (e.g. topic or bucket names), skipping
// empty and non-scalar entries.
func Strings(config map[string]interface{}, key string) []string {
	items, _ := config[key].([]interface{})
	var out []string
	for _, item := range items {
		if v := scalar(item); v != "" {
			out = append(out, v)
		}
	}
	return out
}
