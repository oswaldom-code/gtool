package pubsub

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewPubSubPlugin(t *testing.T) {
	p := NewPubSubPlugin(nil, zap.NewNop())

	assert.NotNil(t, p)
	assert.Equal(t, "pubsub", p.Name())
	assert.NotNil(t, p.httpClient)
}

func TestName(t *testing.T) {
	p := NewPubSubPlugin(nil, nil)
	assert.Equal(t, "pubsub", p.Name())
}

func TestParseConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		p := NewPubSubPlugin(nil, nil)
		got, err := p.parseConfig(map[string]interface{}{})

		require.NoError(t, err)
		assert.Equal(t, defaultImage, got.Image)
		assert.Equal(t, defaultPort, got.Port)
		assert.Equal(t, defaultProjectID, got.ProjectID)
		assert.Empty(t, got.Topics)
		assert.Empty(t, got.Subscriptions)
		assert.Contains(t, got.ContainerName, containerNamePrefix)
	})

	t.Run("topics, subscriptions and numeric port", func(t *testing.T) {
		p := NewPubSubPlugin(nil, nil)
		got, err := p.parseConfig(map[string]interface{}{
			"port":       float64(18085),
			"project-id": "my-project",
			"topics":     []interface{}{"orders", "", 42, "payments"},
			"subscriptions": []interface{}{
				map[string]interface{}{"name": "orders-sub", "topic": "orders"},
				map[string]interface{}{"name": "", "topic": "orders"}, // skipped: no name
				map[string]interface{}{"name": "bad-sub"},             // skipped: no topic
			},
		})

		require.NoError(t, err)
		assert.Equal(t, "18085", got.Port)
		assert.Equal(t, "my-project", got.ProjectID)
		assert.Equal(t, []string{"orders", "payments"}, got.Topics)
		require.Len(t, got.Subscriptions, 1)
		assert.Equal(t, SubscriptionConfig{Name: "orders-sub", Topic: "orders"}, got.Subscriptions[0])
	})

	t.Run("custom container name", func(t *testing.T) {
		p := NewPubSubPlugin(nil, nil)
		got, err := p.parseConfig(map[string]interface{}{"container-name": "my-pubsub"})

		require.NoError(t, err)
		assert.Equal(t, "my-pubsub", got.ContainerName)
	})
}

func TestGetConnectionInfo(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		p := NewPubSubPlugin(nil, nil)
		p.config = &PubSubConfig{Port: "8085", ProjectID: "test-project"}

		got, err := p.GetConnectionInfo()

		require.NoError(t, err)
		assert.Equal(t, "localhost", got.Host)
		assert.Equal(t, 8085, got.Port)
		assert.Equal(t, "http", got.Protocol)
		assert.Equal(t, "localhost:8085", got.Metadata["emulator-host"])
		assert.Equal(t, "test-project", got.Metadata["project-id"])
	})

	t.Run("not launched", func(t *testing.T) {
		p := NewPubSubPlugin(nil, nil)

		_, err := p.GetConnectionInfo()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not launched")
	})
}

func TestResourceURLs(t *testing.T) {
	p := NewPubSubPlugin(nil, nil)
	p.config = &PubSubConfig{Port: "8085", ProjectID: "test-project"}

	assert.Equal(t, "http://localhost:8085/v1/projects/test-project/topics", p.listTopicsURL())
	assert.Equal(t, "http://localhost:8085/v1/projects/test-project/topics/orders", p.topicURL("orders"))
	assert.Equal(t, "http://localhost:8085/v1/projects/test-project/subscriptions/orders-sub", p.subscriptionURL("orders-sub"))
}

func TestIsReady_NotStarted(t *testing.T) {
	p := NewPubSubPlugin(nil, nil)

	ready, err := p.IsReady(nil)

	assert.False(t, ready)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestStop_NoContainer(t *testing.T) {
	p := NewPubSubPlugin(nil, zap.NewNop())

	err := p.Stop(nil)
	assert.NoError(t, err)
}

func TestMustParsePort(t *testing.T) {
	assert.Equal(t, 8085, mustParsePort("8085"))
	assert.Equal(t, 0, mustParsePort("0"))
}
