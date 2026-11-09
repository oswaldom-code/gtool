package ports

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

// seqAllocator hands out consecutive ports starting at first.
func seqAllocator(first int) Allocator {
	next := first
	return func() (int, error) {
		p := next
		next++
		return p, nil
	}
}

func newConfig() *config.Config {
	return &config.Config{
		AppConfig: config.AppConfig{
			Port:        8080,
			Environment: map[string]string{},
		},
		ThirdParty: config.ThirdPartyConfig{
			Mocks: []string{"kafka", "postgresql"},
			MockConfig: map[string]interface{}{
				"postgresql": map[string]interface{}{"port": "auto", "database": "app"},
				"kafka":      map[string]interface{}{"port": "AUTO", "docker-port": "auto", "partitions": 3},
			},
		},
	}
}

func TestAllocate(t *testing.T) {
	t.Run("replaces auto ports deterministically and keeps the rest", func(t *testing.T) {
		cfg := newConfig()
		cfg.AppConfig.HostPort = "auto"

		require.NoError(t, Allocate(cfg, seqAllocator(40000)))

		kafka := cfg.ThirdParty.MockConfig["kafka"].(map[string]interface{})
		pg := cfg.ThirdParty.MockConfig["postgresql"].(map[string]interface{})
		assert.Equal(t, "40000", kafka["docker-port"])
		assert.Equal(t, "40001", kafka["port"])
		assert.Equal(t, 3, kafka["partitions"])
		assert.Equal(t, "40002", pg["port"])
		assert.Equal(t, "app", pg["database"])
		assert.Equal(t, "40003", cfg.AppConfig.HostPort)
	})

	t.Run("explicit ports and non-port keys named auto are untouched", func(t *testing.T) {
		cfg := newConfig()
		cfg.ThirdParty.MockConfig = map[string]interface{}{
			"redis": map[string]interface{}{"port": 6380, "password": "auto"},
			"bad":   "not-a-map",
		}
		cfg.AppConfig.HostPort = "9000"

		require.NoError(t, Allocate(cfg, seqAllocator(40000)))

		redis := cfg.ThirdParty.MockConfig["redis"].(map[string]interface{})
		assert.Equal(t, 6380, redis["port"])
		assert.Equal(t, "auto", redis["password"])
		assert.Equal(t, "9000", cfg.AppConfig.HostPort)
	})

	t.Run("allocator error is returned", func(t *testing.T) {
		cfg := newConfig()
		boom := errors.New("boom")
		err := Allocate(cfg, func() (int, error) { return 0, boom })
		assert.ErrorIs(t, err, boom)
	})

	t.Run("allocator error on app host port is returned", func(t *testing.T) {
		cfg := newConfig()
		cfg.ThirdParty.MockConfig = nil
		cfg.AppConfig.HostPort = "auto"
		boom := errors.New("boom")
		assert.ErrorIs(t, Allocate(cfg, func() (int, error) { return 0, boom }), boom)
	})
}

func TestFreePortAllocator(t *testing.T) {
	alloc := FreePortAllocator()
	seen := map[int]bool{}
	for i := 0; i < 5; i++ {
		p, err := alloc()
		require.NoError(t, err)
		assert.Greater(t, p, 0)
		assert.False(t, seen[p], "port %d returned twice", p)
		seen[p] = true
	}
}

func TestAppHostPort(t *testing.T) {
	cfg := newConfig()
	assert.Equal(t, 8080, AppHostPort(cfg))

	cfg.AppConfig.HostPort = "18080"
	assert.Equal(t, 18080, AppHostPort(cfg))

	cfg.AppConfig.HostPort = "auto"
	assert.Equal(t, 8080, AppHostPort(cfg), "unresolved auto falls back to the container port")
}

func TestValues(t *testing.T) {
	cfg := newConfig()
	cfg.ThirdParty.MockConfig = map[string]interface{}{
		"postgresql": map[string]interface{}{"port": 5433, "database": "app"},
		"kafka":      map[string]interface{}{"port": "19092", "docker-port": "19094"},
		"bad":        "not-a-map",
	}
	cfg.AppConfig.HostPort = "18080"

	assert.Equal(t, map[string]string{
		"mocks.postgresql.port":   "5433",
		"mocks.kafka.port":        "19092",
		"mocks.kafka.docker-port": "19094",
		"app.port":                "8080",
		"app.host-port":           "18080",
	}, Values(cfg))
}

func TestValues_NoAppPort(t *testing.T) {
	cfg := &config.Config{}
	assert.Empty(t, Values(cfg))
}

func TestInterpolate(t *testing.T) {
	values := map[string]string{"mocks.postgresql.port": "40001", "app.port": "8080"}

	t.Run("replaces known placeholders and leaves other text", func(t *testing.T) {
		cfg := newConfig()
		cfg.AppConfig.Environment = map[string]string{
			"DB_PORT": "${mocks.postgresql.port}",
			"DB_URL":  "postgres://host.docker.internal:${mocks.postgresql.port}/app",
			"PORT":    "${app.port}",
			"OTHER":   "${HOME_NOT_OURS}",
			"LITERAL": "plain",
		}

		require.NoError(t, Interpolate(cfg, values))

		assert.Equal(t, "40001", cfg.AppConfig.Environment["DB_PORT"])
		assert.Equal(t, "postgres://host.docker.internal:40001/app", cfg.AppConfig.Environment["DB_URL"])
		assert.Equal(t, "8080", cfg.AppConfig.Environment["PORT"])
		assert.Equal(t, "${HOME_NOT_OURS}", cfg.AppConfig.Environment["OTHER"])
		assert.Equal(t, "plain", cfg.AppConfig.Environment["LITERAL"])
	})

	t.Run("unknown placeholder is a config error", func(t *testing.T) {
		cfg := newConfig()
		cfg.AppConfig.Environment = map[string]string{"REDIS_PORT": "${mocks.redis.port}"}

		err := Interpolate(cfg, values)

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrConfigInvalid))
		assert.Contains(t, err.Error(), "${mocks.redis.port} (in app-config.environment.REDIS_PORT)")
	})
}

func TestResolve(t *testing.T) {
	t.Run("allocates then interpolates", func(t *testing.T) {
		cfg := newConfig()
		cfg.AppConfig.HostPort = "auto"
		cfg.AppConfig.Environment = map[string]string{"DB_PORT": "${mocks.postgresql.port}"}

		values, err := Resolve(cfg, seqAllocator(40000))

		require.NoError(t, err)
		assert.Equal(t, "40002", cfg.AppConfig.Environment["DB_PORT"])
		assert.Equal(t, "40003", values["app.host-port"])
	})

	t.Run("allocation error stops resolution", func(t *testing.T) {
		_, err := Resolve(newConfig(), func() (int, error) { return 0, errors.New("boom") })
		assert.Error(t, err)
	})

	t.Run("interpolation error is returned", func(t *testing.T) {
		cfg := newConfig()
		cfg.AppConfig.Environment = map[string]string{"X": "${mocks.nope.port}"}
		_, err := Resolve(cfg, seqAllocator(40000))
		assert.Error(t, err)
	})
}

func TestTestEnv(t *testing.T) {
	env := TestEnv(map[string]string{
		"mocks.postgresql.port":   "40001",
		"mocks.kafka.docker-port": "40002",
		"app.port":                "8080",
		"app.host-port":           "40003",
	})

	assert.Equal(t, map[string]string{
		"GTOOL_POSTGRESQL_PORT":   "40001",
		"GTOOL_KAFKA_DOCKER_PORT": "40002",
		"GTOOL_APP_PORT":          "8080",
		"GTOOL_APP_HOST_PORT":     "40003",
		"GTOOL_APP_URL":           "http://localhost:40003",
	}, env)
}

func TestNoAuto(t *testing.T) {
	cfg := newConfig()

	err := Allocate(cfg, NoAuto)

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrConfigInvalid))
	assert.Contains(t, err.Error(), "gtool test")
}

func TestValues_UsesPluginDefaultsForEnabledMocks(t *testing.T) {
	cfg := &config.Config{ThirdParty: config.ThirdPartyConfig{
		Mocks: []string{"postgresql", "kafka"},
		MockConfig: map[string]interface{}{
			"kafka": map[string]interface{}{"port": "19092"},
			"redis": map[string]interface{}{"password": "x"},
		},
	}}

	values := Values(cfg)

	assert.Equal(t, "5432", values["mocks.postgresql.port"], "default when not configured")
	assert.Equal(t, "19092", values["mocks.kafka.port"], "configured value wins")
	assert.Equal(t, "9094", values["mocks.kafka.docker-port"])
	assert.NotContains(t, values, "mocks.redis.port", "mocks that are not enabled have no defaults")
}

func TestResolve_PlaceholderOnDefaultPort(t *testing.T) {
	cfg := &config.Config{
		AppConfig:  config.AppConfig{Environment: map[string]string{"DB_PORT": "${mocks.postgresql.port}"}},
		ThirdParty: config.ThirdPartyConfig{Mocks: []string{"postgresql"}},
	}

	_, err := Resolve(cfg, seqAllocator(40000))

	require.NoError(t, err)
	assert.Equal(t, "5432", cfg.AppConfig.Environment["DB_PORT"])
}

func TestAllocate_RejectsInvalidHostPort(t *testing.T) {
	for _, hp := range []string{"abc", "0", "70000"} {
		cfg := newConfig()
		cfg.AppConfig.HostPort = hp
		err := Allocate(cfg, seqAllocator(40000))
		require.Error(t, err, hp)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrConfigInvalid), hp)
	}
}

func TestValues_NoAppEntriesWhenNotPublished(t *testing.T) {
	cfg := &config.Config{AppConfig: config.AppConfig{HostPort: "41000"}}
	values := Values(cfg)
	assert.NotContains(t, values, "app.host-port", "port 0 publishes nothing")
	assert.NotContains(t, values, "app.port")
}
