package server

import (
	"net/http"

	"github.com/johndemoss/o11y-workbench/internal/ottlengine"
)

// ensureEngines returns the engine registry, lazily creating an in-process
// default when tests construct a bare Server without calling New.
func (s *Server) ensureEngines() *ottlengine.Registry {
	if s.engines == nil {
		s.engines = ottlengine.NewRegistry(ottlengine.DefaultContribVersion, &ottlengine.LocalEngine{})
	}
	return s.engines
}

// resolveEngine picks an OTTL engine: query collector_version > bodyVersion > registry default.
func (s *Server) resolveEngine(r *http.Request, bodyVersion string) (ottlengine.Engine, error) {
	v := r.URL.Query().Get("collector_version")
	if v == "" {
		v = bodyVersion
	}
	return s.ensureEngines().Get(v)
}
