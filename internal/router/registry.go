package router

import (
	"fmt"
	"sync"
)

// Registry manages backends with capability awareness.
// It holds both chat/embed backends and TTS backends, and supports
// querying by name, capability, or model.
type Registry struct {
	mu       sync.RWMutex
	backends map[string]BackendInfo
	routes   []ModelRoute
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		backends: make(map[string]BackendInfo),
	}
}

// Register adds a BackendInfo to the registry.
func (r *Registry) Register(info BackendInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.backends[info.Name] = info

	// Build model routes from the backend's models.
	for _, m := range info.Models {
		r.routes = append(r.routes, ModelRoute{
			Model:       m.ID,
			Capability:  m.Kind,
			BackendName: info.Name,
		})
	}
}

// Get returns a BackendInfo by name.
func (r *Registry) Get(name string) (BackendInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, ok := r.backends[name]
	return info, ok
}

// All returns all registered BackendInfo entries.
func (r *Registry) All() []BackendInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]BackendInfo, 0, len(r.backends))
	for _, info := range r.backends {
		result = append(result, info)
	}
	return result
}

// Len returns the number of registered backends.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.backends)
}

// BackendsByCapability returns all backends that support the given capability.
func (r *Registry) BackendsByCapability(kind Capability) []BackendInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []BackendInfo
	for _, info := range r.backends {
		for _, c := range info.Capabilities {
			if c == kind {
				result = append(result, info)
				break
			}
		}
	}
	return result
}

// ErrBackendNotFound is returned when no matching backend is found.
var ErrBackendNotFound = fmt.Errorf("no backend found")

// ErrCapabilityNotSupported is returned when no backend supports the requested capability.
var ErrCapabilityNotSupported = fmt.Errorf("capability not supported by any backend")
