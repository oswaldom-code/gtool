package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/internal/plugin/services/postgresql"
)

func TestDefaultPorts_CoversEveryRegisteredService(t *testing.T) {
	reg := plugin.NewRegistry()
	require.NoError(t, RegisterAll(reg, nil, zap.NewNop()))

	defaults := DefaultPorts()
	assert.ElementsMatch(t, reg.ListServices(), keys(defaults), "every plugin must declare its default ports")
	for svc, ports := range defaults {
		assert.NotEmpty(t, ports, svc)
	}
	assert.Equal(t, map[string]string{"port": "9092", "docker-port": "9094"}, defaults["kafka"])
	assert.Equal(t, map[string]string{"port": "5432"}, defaults["postgresql"])
}

func keys(m map[string]map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestConfigKeys_CoverEveryServiceAndItsPorts(t *testing.T) {
	keysBySvc := ConfigKeys()
	assert.ElementsMatch(t, keys(DefaultPorts()), mapKeys(keysBySvc))
	for svc, ports := range DefaultPorts() {
		for port := range ports {
			assert.Contains(t, keysBySvc[svc], port, "%s must accept its port key %s", svc, port)
		}
		assert.Contains(t, keysBySvc[svc], "image", svc)
		assert.Contains(t, keysBySvc[svc], "container-name", svc)
	}
}

func mapKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestRegisterAll_IsAllOrNothing(t *testing.T) {
	reg := plugin.NewRegistry()
	require.NoError(t, reg.RegisterService(postgresql.NewPostgreSQLPlugin(nil, zap.NewNop())))

	err := RegisterAll(reg, nil, zap.NewNop())

	require.Error(t, err)
	assert.Equal(t, []string{"postgresql"}, reg.ListServices(), "nothing else may be registered after a conflict")
}
