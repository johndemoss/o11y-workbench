package ottlengine

import (
	"fmt"
	"sort"
	"sync"
)

// Registry maps collector contrib versions to Engine backends.
type Registry struct {
	mu             sync.RWMutex
	defaultVersion string
	engines        map[string]Engine
}

// NewRegistry creates a registry with the given default engine.
func NewRegistry(defaultVersion string, defaultEngine Engine) *Registry {
	if defaultVersion == "" {
		defaultVersion = DefaultContribVersion
	}
	r := &Registry{
		defaultVersion: defaultVersion,
		engines:        make(map[string]Engine),
	}
	if defaultEngine != nil {
		r.engines[defaultVersion] = defaultEngine
	}
	return r
}

// Default returns the default collector contrib version.
func (r *Registry) Default() string {
	return r.defaultVersion
}

// Register adds or replaces an engine for a contrib version.
func (r *Registry) Register(version string, eng Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.engines == nil {
		r.engines = make(map[string]Engine)
	}
	r.engines[version] = eng
}

// List returns VersionInfo for every registered engine, sorted by contrib version.
func (r *Registry) List() []VersionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.engines) == 0 {
		return nil
	}
	keys := make([]string, 0, len(r.engines))
	for v, eng := range r.engines {
		if engineExited(eng) {
			continue
		}
		keys = append(keys, v)
	}
	sort.Strings(keys)
	out := make([]VersionInfo, 0, len(keys))
	for _, v := range keys {
		out = append(out, r.engines[v].Info())
	}
	return out
}

// Get returns the engine for version. Empty version resolves to the default.
// Unknown versions return an error of the form "unsupported collector_version: %s".
func (r *Registry) Get(version string) (Engine, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if version == "" {
		version = r.defaultVersion
	}
	eng, ok := r.engines[version]
	if !ok || engineExited(eng) {
		return nil, fmt.Errorf("unsupported collector_version: %s", version)
	}
	return eng, nil
}

// supervisedEngine is implemented by engines backed by a process whose exit is
// observed by the supervisor's reaper.
type supervisedEngine interface {
	exited() bool
}

// engineExited withdraws reaped worker engines from List and Get on every call,
// so a dead train is never advertised or resolved.
func engineExited(eng Engine) bool {
	s, ok := eng.(supervisedEngine)
	return ok && s.exited()
}
