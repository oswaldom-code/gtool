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
