package kafka

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewKafkaPlugin(t *testing.T) {
	p := NewKafkaPlugin(nil, zap.NewNop())

	assert.NotNil(t, p)
	assert.Equal(t, "kafka", p.Name())
}

func TestName(t *testing.T) {
	p := NewKafkaPlugin(nil, nil)
	assert.Equal(t, "kafka", p.Name())
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name           string
		input          map[string]interface{}
		wantImage      string
		wantPort       string
		wantPartitions int
		wantTopics     []string
	}{
		{
			name:           "default config",
			input:          map[string]interface{}{},
			wantImage:      defaultImage,
			wantPort:       defaultPort,
			wantPartitions: defaultPartitions,
			wantTopics:     nil,
		},
		{
			name: "custom config with topics",
			input: map[string]interface{}{
				"image":      "apache/kafka:3.6.0",
				"port":       "19092",
				"partitions": float64(3),
				"topics":     []interface{}{"orders", "payments"},
			},
			wantImage:      "apache/kafka:3.6.0",
			wantPort:       "19092",
			wantPartitions: 3,
			wantTopics:     []string{"orders", "payments"},
		},
		{
			name: "numeric port",
			input: map[string]interface{}{
				"port": float64(19092),
			},
			wantImage:      defaultImage,
			wantPort:       "19092",
			wantPartitions: defaultPartitions,
		},
		{
			name: "topics list ignores non-string and empty entries",
			input: map[string]interface{}{
				"topics": []interface{}{"orders", "", 42, "events"},
			},
			wantImage:      defaultImage,
			wantPort:       defaultPort,
			wantPartitions: defaultPartitions,
			wantTopics:     []string{"orders", "events"},
		},
		{
			name: "zero partitions falls back to default",
			input: map[string]interface{}{
				"partitions": float64(0),
			},
			wantImage:      defaultImage,
			wantPort:       defaultPort,
			wantPartitions: defaultPartitions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewKafkaPlugin(nil, nil)
			got, err := p.parseConfig(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.wantImage, got.Image)
			assert.Equal(t, tt.wantPort, got.Port)
			assert.Equal(t, tt.wantPartitions, got.Partitions)
			assert.Equal(t, tt.wantTopics, got.Topics)
			assert.Contains(t, got.ContainerName, containerNamePrefix)
		})
	}
}

func TestParseConfig_CustomContainerName(t *testing.T) {
	p := NewKafkaPlugin(nil, nil)
	got, err := p.parseConfig(map[string]interface{}{"container-name": "my-kafka"})

	require.NoError(t, err)
	assert.Equal(t, "my-kafka", got.ContainerName)
}

func TestGetConnectionInfo(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		p := NewKafkaPlugin(nil, nil)
		p.config = &KafkaConfig{Port: "9092"}

		got, err := p.GetConnectionInfo()

		require.NoError(t, err)
		assert.Equal(t, "localhost", got.Host)
		assert.Equal(t, 9092, got.Port)
		assert.Equal(t, "kafka", got.Protocol)
		assert.Equal(t, "localhost:9092", got.Metadata["bootstrap-servers"])
	})

	t.Run("not launched", func(t *testing.T) {
		p := NewKafkaPlugin(nil, nil)

		_, err := p.GetConnectionInfo()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not launched")
	})
}

func TestBrokerEnv(t *testing.T) {
	p := NewKafkaPlugin(nil, nil)
	env := p.brokerEnv(&KafkaConfig{Port: "19092"})

	// External listener must be advertised on the configured host port.
	assert.Contains(t, env, "KAFKA_ADVERTISED_LISTENERS=INTERNAL://localhost:29092,EXTERNAL://localhost:19092")
	assert.Contains(t, env, "KAFKA_PROCESS_ROLES=broker,controller")
	assert.Contains(t, env, "KAFKA_INTER_BROKER_LISTENER_NAME=INTERNAL")
}

func TestBootstrapServers(t *testing.T) {
	p := NewKafkaPlugin(nil, nil)
	p.config = &KafkaConfig{Port: "19092"}

	assert.Equal(t, "localhost:19092", p.bootstrapServers())
}

func TestIsReady_NotStarted(t *testing.T) {
	p := NewKafkaPlugin(nil, nil)

	ready, err := p.IsReady(nil)

	assert.False(t, ready)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestStop_NoContainer(t *testing.T) {
	p := NewKafkaPlugin(nil, zap.NewNop())

	err := p.Stop(nil)
	assert.NoError(t, err)
}

func TestMustParsePort(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"9092", 9092},
		{"19092", 19092},
		{"0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, mustParsePort(tt.input))
		})
	}
}
