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

type mockAppLauncher struct {
	tech string
}

func (m *mockAppLauncher) Technology() string                                  { return m.tech }
func (m *mockAppLauncher) Launch(ctx context.Context, config *AppConfig) error { return nil }
func (m *mockAppLauncher) IsReady(ctx context.Context) (bool, error)           { return true, nil }
func (m *mockAppLauncher) Stop(ctx context.Context) error                      { return nil }
func (m *mockAppLauncher) Restart(ctx context.Context) error                   { return nil }
func (m *mockAppLauncher) GetPID() (int, error)                                { return 1234, nil }

type mockTestExecutor struct {
	framework string
}

func (m *mockTestExecutor) Framework() string { return m.framework }
func (m *mockTestExecutor) Execute(ctx context.Context, config *TestConfig) (*TestResult, error) {
	return &TestResult{Total: 10, Passed: 10}, nil
}
func (m *mockTestExecutor) Cancel(ctx context.Context) error { return nil }
func (m *mockTestExecutor) GetProgress(ctx context.Context) (*TestProgress, error) {
	return &TestProgress{Current: 5, Total: 10}, nil
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	assert.NotNil(t, registry)
	assert.NotNil(t, registry.services)
	assert.NotNil(t, registry.launchers)
	assert.NotNil(t, registry.executors)
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
	registry.RegisterService(mock)

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
			registry.RegisterService(mock)
		}

		services := registry.ListServices()
		assert.Len(t, services, 3)
		assert.Contains(t, services, "couchbase")
		assert.Contains(t, services, "kafka")
		assert.Contains(t, services, "postgresql")
	})
}

func TestRegistry_RegisterLauncher(t *testing.T) {
	registry := NewRegistry()

	t.Run("register new launcher", func(t *testing.T) {
		mock := &mockAppLauncher{tech: "golang"}
		err := registry.RegisterLauncher(mock)
		assert.NoError(t, err)
	})

	t.Run("register duplicate launcher", func(t *testing.T) {
		mock := &mockAppLauncher{tech: "golang"}
		err := registry.RegisterLauncher(mock)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("register multiple launchers", func(t *testing.T) {
		launchers := []*mockAppLauncher{
			{tech: "nodejs"},
			{tech: "generic"},
		}

		for _, launcher := range launchers {
			err := registry.RegisterLauncher(launcher)
			assert.NoError(t, err)
		}
	})
}

func TestRegistry_GetLauncher(t *testing.T) {
	registry := NewRegistry()
	mock := &mockAppLauncher{tech: "nodejs"}
	registry.RegisterLauncher(mock)

	t.Run("get existing launcher", func(t *testing.T) {
		launcher, err := registry.GetLauncher("nodejs")
		assert.NoError(t, err)
		assert.NotNil(t, launcher)
		assert.Equal(t, "nodejs", launcher.Technology())
	})

	t.Run("get non-existent launcher", func(t *testing.T) {
		launcher, err := registry.GetLauncher("rust")
		assert.Error(t, err)
		assert.Nil(t, launcher)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRegistry_RegisterExecutor(t *testing.T) {
	registry := NewRegistry()

	t.Run("register new executor", func(t *testing.T) {
		mock := &mockTestExecutor{framework: "karate"}
		err := registry.RegisterExecutor(mock)
		assert.NoError(t, err)
	})

	t.Run("register duplicate executor", func(t *testing.T) {
		mock := &mockTestExecutor{framework: "karate"}
		err := registry.RegisterExecutor(mock)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("register multiple executors", func(t *testing.T) {
		mock := &mockTestExecutor{framework: "cypress"}
		err := registry.RegisterExecutor(mock)
		assert.NoError(t, err)
	})
}

func TestRegistry_GetExecutor(t *testing.T) {
	registry := NewRegistry()
	mock := &mockTestExecutor{framework: "test-launcher-back"}
	registry.RegisterExecutor(mock)

	t.Run("get existing executor", func(t *testing.T) {
		executor, err := registry.GetExecutor("test-launcher-back")
		assert.NoError(t, err)
		assert.NotNil(t, executor)
		assert.Equal(t, "test-launcher-back", executor.Framework())
	})

	t.Run("get non-existent executor", func(t *testing.T) {
		executor, err := registry.GetExecutor("jest")
		assert.Error(t, err)
		assert.Nil(t, executor)
		assert.Contains(t, err.Error(), "not found")
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
				registry.RegisterService(mock)
				done <- true
			}(i)
		}

		// Concurrent retrievals
		for i := 0; i < 5; i++ {
			go func(id int) {
				registry.GetService(fmt.Sprintf("service%d", id))
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

	// Register all types of plugins
	service := &mockServicePlugin{name: "couchbase"}
	launcher := &mockAppLauncher{tech: "golang"}
	executor := &mockTestExecutor{framework: "karate"}

	err := registry.RegisterService(service)
	require.NoError(t, err)

	err = registry.RegisterLauncher(launcher)
	require.NoError(t, err)

	err = registry.RegisterExecutor(executor)
	require.NoError(t, err)

	// Retrieve and verify
	retrievedService, err := registry.GetService("couchbase")
	require.NoError(t, err)
	assert.Equal(t, service, retrievedService)

	retrievedLauncher, err := registry.GetLauncher("golang")
	require.NoError(t, err)
	assert.Equal(t, launcher, retrievedLauncher)

	retrievedExecutor, err := registry.GetExecutor("karate")
	require.NoError(t, err)
	assert.Equal(t, executor, retrievedExecutor)
}

func BenchmarkRegistry_RegisterService(b *testing.B) {
	registry := NewRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mock := &mockServicePlugin{name: fmt.Sprintf("service%d", i)}
		registry.RegisterService(mock)
	}
}

func BenchmarkRegistry_GetService(b *testing.B) {
	registry := NewRegistry()
	mock := &mockServicePlugin{name: "test"}
	registry.RegisterService(mock)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.GetService("test")
	}
}

func BenchmarkRegistry_ListServices(b *testing.B) {
	registry := NewRegistry()

	// Register 100 services
	for i := 0; i < 100; i++ {
		mock := &mockServicePlugin{name: fmt.Sprintf("service%d", i)}
		registry.RegisterService(mock)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.ListServices()
	}
}
