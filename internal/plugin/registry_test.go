package plugin

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockServicePlugin struct {
	name string
}

func (m *mockServicePlugin) Name() string { return m.name }
func (m *mockServicePlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	return nil
}
func (m *mockServicePlugin) IsReady(ctx context.Context) (bool, error) { return true, nil }
func (m *mockServicePlugin) Stop(ctx context.Context) error            { return nil }
func (m *mockServicePlugin) GetConnectionInfo() (*ConnectionInfo, error) {
	return &ConnectionInfo{Host: "localhost", Port: 8080}, nil
}
func (m *mockServicePlugin) GetLogs(ctx context.Context, opts *LogOptions) ([]string, error) {
	return []string{"log1", "log2"}, nil
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	assert.NotNil(t, registry)
	assert.NotNil(t, registry.services)
}

func TestRegistry_RegisterService(t *testing.T) {
	registry := NewRegistry()

	t.Run("register new service", func(t *testing.T) {
		mock := &mockServicePlugin{name: "couchbase"}
		err := registry.RegisterService(mock)
		assert.NoError(t, err)
	})

	t.Run("register duplicate service", func(t *testing.T) {
		mock := &mockServicePlugin{name: "couchbase"}
		err := registry.RegisterService(mock)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("register multiple services", func(t *testing.T) {
		mocks := []*mockServicePlugin{
			{name: "kafka"},
			{name: "postgresql"},
			{name: "pubsub"},
		}

		for _, mock := range mocks {
			err := registry.RegisterService(mock)
			assert.NoError(t, err)
		}
	})
}

func TestRegistry_GetService(t *testing.T) {
	registry := NewRegistry()
	mock := &mockServicePlugin{name: "mountebank"}
	require.NoError(t, registry.RegisterService(mock))

	t.Run("get existing service", func(t *testing.T) {
		service, err := registry.GetService("mountebank")
		assert.NoError(t, err)
		assert.NotNil(t, service)
		assert.Equal(t, "mountebank", service.Name())
	})

	t.Run("get non-existent service", func(t *testing.T) {
		service, err := registry.GetService("nonexistent")
		assert.Error(t, err)
		assert.Nil(t, service)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRegistry_ListServices(t *testing.T) {
	registry := NewRegistry()

	t.Run("list empty registry", func(t *testing.T) {
		services := registry.ListServices()
		assert.NotNil(t, services)
		assert.Empty(t, services)
	})

	t.Run("list multiple services", func(t *testing.T) {
		mocks := []*mockServicePlugin{
			{name: "couchbase"},
			{name: "kafka"},
			{name: "postgresql"},
		}

		for _, mock := range mocks {
			require.NoError(t, registry.RegisterService(mock))
		}

		services := registry.ListServices()
		assert.Len(t, services, 3)
		assert.Contains(t, services, "couchbase")
		assert.Contains(t, services, "kafka")
		assert.Contains(t, services, "postgresql")
	})
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewRegistry()

	// Test concurrent service registration and retrieval
	t.Run("concurrent service operations", func(t *testing.T) {
		done := make(chan bool, 10)

		// Concurrent registrations
		for i := 0; i < 5; i++ {
			go func(id int) {
				mock := &mockServicePlugin{name: fmt.Sprintf("service%d", id)}
				assert.NoError(t, registry.RegisterService(mock))
				done <- true
			}(i)
		}

		// Concurrent retrievals
		for i := 0; i < 5; i++ {
			go func(id int) {
				_, _ = registry.GetService(fmt.Sprintf("service%d", id))
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify all services were registered
		services := registry.ListServices()
		assert.GreaterOrEqual(t, len(services), 5)
	})
}

func TestRegistry_FullWorkflow(t *testing.T) {
	registry := NewRegistry()

	service := &mockServicePlugin{name: "couchbase"}

	err := registry.RegisterService(service)
	require.NoError(t, err)

	// Retrieve and verify
	retrievedService, err := registry.GetService("couchbase")
	require.NoError(t, err)
	assert.Equal(t, service, retrievedService)
}

func BenchmarkRegistry_RegisterService(b *testing.B) {
	registry := NewRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mock := &mockServicePlugin{name: fmt.Sprintf("service%d", i)}
		if err := registry.RegisterService(mock); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRegistry_GetService(b *testing.B) {
	registry := NewRegistry()
	mock := &mockServicePlugin{name: "test"}
	if err := registry.RegisterService(mock); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := registry.GetService("test"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRegistry_ListServices(b *testing.B) {
	registry := NewRegistry()

	// Register 100 services
	for i := 0; i < 100; i++ {
		mock := &mockServicePlugin{name: fmt.Sprintf("service%d", i)}
		if err := registry.RegisterService(mock); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.ListServices()
	}
}

func TestRegistry_RejectsNilPlugin(t *testing.T) {
	r := NewRegistry()
	assert.Error(t, r.RegisterService(nil))
}
