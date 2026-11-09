package svcconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPort(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]interface{}
		want   string
	}{
		{name: "string", config: map[string]interface{}{"port": "5433"}, want: "5433"},
		{name: "yaml int", config: map[string]interface{}{"port": 5433}, want: "5433"},
		{name: "int64", config: map[string]interface{}{"port": int64(5433)}, want: "5433"},
		{name: "uint64", config: map[string]interface{}{"port": uint64(5433)}, want: "5433"},
		{name: "json float64", config: map[string]interface{}{"port": float64(5433)}, want: "5433"},
		{name: "empty string falls back", config: map[string]interface{}{"port": ""}, want: "5432"},
		{name: "missing key falls back", config: map[string]interface{}{}, want: "5432"},
		{name: "nil config falls back", config: nil, want: "5432"},
		{name: "unsupported type falls back", config: map[string]interface{}{"port": true}, want: "5432"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Port(tt.config, "port", "5432"))
		})
	}
}

func TestString(t *testing.T) {
	cfg := map[string]interface{}{"password": 123456, "project": float64(987), "name": "x", "empty": "", "bad": []int{1}}
	assert.Equal(t, "123456", String(cfg, "password", "default"), "unquoted YAML numbers are values, not ignored")
	assert.Equal(t, "987", String(cfg, "project", ""))
	assert.Equal(t, "x", String(cfg, "name", ""))
	assert.Equal(t, "default", String(cfg, "empty", "default"))
	assert.Equal(t, "default", String(cfg, "bad", "default"))
	assert.Equal(t, "default", String(cfg, "missing", "default"))
}

func TestStrings(t *testing.T) {
	cfg := map[string]interface{}{"topics": []interface{}{"orders", 42, "", true, float64(7)}, "notalist": "x"}
	assert.Equal(t, []string{"orders", "42", "7"}, Strings(cfg, "topics"))
	assert.Nil(t, Strings(cfg, "notalist"))
	assert.Nil(t, Strings(cfg, "missing"))
}
