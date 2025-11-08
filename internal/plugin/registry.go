package plugin

import (
	"fmt"
	"sync"
)

type Registry struct {
	services  map[string]ServicePlugin
	launchers map[string]AppLauncher
	executors map[string]TestExecutor
	mu        sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		services:  make(map[string]ServicePlugin),
		launchers: make(map[string]AppLauncher),
		executors: make(map[string]TestExecutor),
	}
}

func (r *Registry) RegisterService(plugin ServicePlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := plugin.Name()
	if _, exists := r.services[name]; exists {
		return fmt.Errorf("service plugin %s already registered", name)
	}

	r.services[name] = plugin
	return nil
}

func (r *Registry) GetService(name string) (ServicePlugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, exists := r.services[name]
	if !exists {
		return nil, fmt.Errorf("service plugin %s not found", name)
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

func (r *Registry) RegisterLauncher(plugin AppLauncher) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tech := plugin.Technology()
	if _, exists := r.launchers[tech]; exists {
		return fmt.Errorf("app launcher %s already registered", tech)
	}

	r.launchers[tech] = plugin
	return nil
}

func (r *Registry) GetLauncher(technology string) (AppLauncher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, exists := r.launchers[technology]
	if !exists {
		return nil, fmt.Errorf("app launcher %s not found", technology)
	}

	return plugin, nil
}

func (r *Registry) RegisterExecutor(plugin TestExecutor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	framework := plugin.Framework()
	if _, exists := r.executors[framework]; exists {
		return fmt.Errorf("test executor %s already registered", framework)
	}

	r.executors[framework] = plugin
	return nil
}

func (r *Registry) GetExecutor(framework string) (TestExecutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, exists := r.executors[framework]
	if !exists {
		return nil, fmt.Errorf("test executor %s not found", framework)
	}

	return plugin, nil
}
