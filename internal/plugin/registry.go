package plugin

import (
	"fmt"
	"sync"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

type Registry struct {
	services map[string]ServicePlugin
	mu       sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		services: make(map[string]ServicePlugin),
	}
}

func (r *Registry) RegisterService(plugin ServicePlugin) error {
	if plugin == nil {
		return gtErrors.New(gtErrors.ErrInvalidArgument, "cannot register a nil ServicePlugin")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	name := plugin.Name()
	if _, exists := r.services[name]; exists {
		return gtErrors.New(gtErrors.ErrInvalidArgument, fmt.Sprintf("service plugin %s already registered", name))
	}

	r.services[name] = plugin
	return nil
}

func (r *Registry) GetService(name string) (ServicePlugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, exists := r.services[name]
	if !exists {
		return nil, gtErrors.New(gtErrors.ErrInvalidArgument, fmt.Sprintf("service plugin %s not found", name))
	}

	return plugin, nil
}

func (r *Registry) ListServices() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.services))
	for name := range r.services {
		names = append(names, name)
	}
	return names
}
