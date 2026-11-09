// Package svcconfig holds parsing helpers shared by the mock service plugins.
package svcconfig

import "fmt"

// Port reads a port-like config value as a string. YAML decodes unquoted
// numbers as int and JSON as float64, so every numeric form is accepted.
// It returns fallback when the key is absent, empty or of another type.
func Port(config map[string]interface{}, key, fallback string) string {
	switch v := config[key].(type) {
	case string:
		if v != "" {
			return v
		}
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case uint64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	}
	return fallback
}
