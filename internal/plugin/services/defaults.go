package services

import (
	"sort"

	"github.com/oswaldom-code/gtool/internal/plugin/services/couchbase"
	"github.com/oswaldom-code/gtool/internal/plugin/services/floci"
	"github.com/oswaldom-code/gtool/internal/plugin/services/gcs"
	"github.com/oswaldom-code/gtool/internal/plugin/services/kafka"
	"github.com/oswaldom-code/gtool/internal/plugin/services/mongodb"
	"github.com/oswaldom-code/gtool/internal/plugin/services/mountebank"
	"github.com/oswaldom-code/gtool/internal/plugin/services/mysql"
	"github.com/oswaldom-code/gtool/internal/plugin/services/postgresql"
	"github.com/oswaldom-code/gtool/internal/plugin/services/pubsub"
	"github.com/oswaldom-code/gtool/internal/plugin/services/redis"
)

// DefaultPorts returns, per registered service name, the default host port of
// every port key the plugin accepts.
func DefaultPorts() map[string]map[string]string {
	return map[string]map[string]string{
		"couchbase":  couchbase.DefaultPorts(),
		"floci":      floci.DefaultPorts(),
		"gcs":        gcs.DefaultPorts(),
		"kafka":      kafka.DefaultPorts(),
		"mongodb":    mongodb.DefaultPorts(),
		"mountebank": mountebank.DefaultPorts(),
		"mysql":      mysql.DefaultPorts(),
		"postgresql": postgresql.DefaultPorts(),
		"pubsub":     pubsub.DefaultPorts(),
		"redis":      redis.DefaultPorts(),
	}
}

// ConfigKeys returns, per registered service name, the mock-config keys the
// plugin accepts.
func ConfigKeys() map[string][]string {
	return map[string][]string{
		"couchbase":  couchbase.ConfigKeys(),
		"floci":      floci.ConfigKeys(),
		"gcs":        gcs.ConfigKeys(),
		"kafka":      kafka.ConfigKeys(),
		"mongodb":    mongodb.ConfigKeys(),
		"mountebank": mountebank.ConfigKeys(),
		"mysql":      mysql.ConfigKeys(),
		"postgresql": postgresql.ConfigKeys(),
		"pubsub":     pubsub.ConfigKeys(),
		"redis":      redis.ConfigKeys(),
	}
}

// Names returns the registered service names, sorted.
func Names() []string {
	names := make([]string, 0, len(ConfigKeys()))
	for name := range ConfigKeys() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
