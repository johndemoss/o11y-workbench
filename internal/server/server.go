package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/ottlengine"
)

//go:embed web
var webFS embed.FS

//go:embed pipeline-ui/dist
var pipelineFS embed.FS

type Server struct {
	store           *RunStore
	mux             *http.ServeMux
	httpSrv         *http.Server
	addr            string
	pipelineEnabled bool
	remotetap       *RemotetapManager
	engines         *ottlengine.Registry
	engineStops     []func() error
}

type serverConfig struct {
	addr            string
	pipelineEnabled bool
}

func New(addr string, pipelineEnabled bool) (*Server, error) {
	return newServer(serverConfig{addr: addr, pipelineEnabled: pipelineEnabled})
}

func newServer(cfg serverConfig) (*Server, error) {
	store := NewRunStore()

	engines := ottlengine.NewRegistry(ottlengine.DefaultContribVersion, &ottlengine.LocalEngine{})
	workers, stops := ottlengine.StartConfiguredWorkers(context.Background())
	for ver, eng := range workers {
		engines.Register(ver, eng)
	}

	s := &Server{
		store:           store,
		mux:             http.NewServeMux(),
		addr:            cfg.addr,
		pipelineEnabled: cfg.pipelineEnabled,
		remotetap:       NewRemotetapManager(store),
		engines:         engines,
		engineStops:     stops,
	}
	if err := s.registerRoutes(); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("register routes: %w", err)
	}
	return s, nil
}

func (s *Server) registerRoutes() error {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/collector-versions", s.handleCollectorVersions)
	s.mux.HandleFunc("GET /api/scenarios", s.handleScenarios)
	s.mux.HandleFunc("POST /api/scenarios", s.handleUploadScenarios)
	s.mux.HandleFunc("GET /api/config/defaults", s.handleDefaults)
	s.mux.HandleFunc("POST /api/runs", s.handleStartRun)
	s.mux.HandleFunc("GET /api/runs", s.handleListRuns)
	s.mux.HandleFunc("GET /api/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("POST /api/runs/import", s.handleImportRun)
	s.mux.HandleFunc("POST /api/runs/{id}/stop", s.handleStopRun)
	s.mux.HandleFunc("POST /api/runs/{id}/pause", s.handlePauseRun)
	s.mux.HandleFunc("POST /api/runs/{id}/resume", s.handleResumeRun)
	s.mux.HandleFunc("POST /api/ottl/validate", s.handleOTTLValidate)
	s.mux.HandleFunc("POST /api/ottl/transform", s.handleOTTLTransform)
	s.mux.HandleFunc("POST /api/ottl/filter", s.handleOTTLFilter)
	s.mux.HandleFunc("POST /api/ottl/filter-validate", s.handleOTTLFilterValidate)

	s.mux.HandleFunc("POST /api/remotetap/connect", s.handleRemotetapConnect)
	s.mux.HandleFunc("POST /api/remotetap/disconnect", s.handleRemotetapDisconnect)
	s.mux.HandleFunc("GET /api/remotetap/status", s.handleRemotetapStatus)
	s.mux.HandleFunc("GET /api/remotetap/usage/{runID}", s.handleRemotetapUsage)
	s.mux.HandleFunc("POST /api/remotetap/stop/{runID}", s.handleRemotetapStop)

	if s.pipelineEnabled {
		s.mux.HandleFunc("POST /api/ottl/pipeline/validate", s.handlePipelineValidate)
		s.mux.HandleFunc("POST /api/ottl/pipeline/execute", s.handlePipelineExecute)
		s.mux.HandleFunc("POST /api/ottl/pipeline/import-config", s.handlePipelineImport)
		s.mux.HandleFunc("POST /api/ottl/pipeline/export", s.handlePipelineExport)
		s.mux.HandleFunc("POST /api/ottl/pipeline/diff", s.handlePipelineDiff)
		s.mux.HandleFunc("POST /api/ottl/pipeline/topological-sort", s.handleTopologicalSort)
		s.mux.HandleFunc("POST /api/ottl/pipeline/would-cycle", s.handleWouldCycle)
		s.mux.HandleFunc("POST /api/ottl/pipeline/from-yaml", s.handleFromYaml)

		pipelineSub, err := fs.Sub(pipelineFS, "pipeline-ui/dist")
		if err != nil {
			return fmt.Errorf("embed pipeline ui fs: %w", err)
		}
		pipelineServer := http.FileServer(http.FS(pipelineSub))
		s.mux.Handle("GET /pipeline/", http.StripPrefix("/pipeline/", pipelineServer))
	}

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return fmt.Errorf("embed web fs: %w", err)
	}
	fileServer := http.FileServer(http.FS(sub))
	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err == nil {
			if path == "index.html" {
				w.Header().Set("Cache-Control", "no-cache, must-revalidate")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
	return nil
}

func (s *Server) Store() *RunStore { return s.store }

// Close releases evaluation coordinator resources and stops OTTL worker subprocesses.
// Safe to call more than once.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	if s.evalSequence != nil {
		s.evalSequence.Close()
		s.evalSequence = nil
	}
	if s.evaluation != nil {
		s.evaluation.Close()
		s.evaluation = nil
	}
	var first error
	for _, stop := range s.engineStops {
		if stop == nil {
			continue
		}
		if err := stop(); err != nil && first == nil {
			first = err
		}
	}
	s.engineStops = nil
	return first
}

// Shutdown gracefully stops the HTTP server (if running) and OTTL workers.
func (s *Server) Shutdown(ctx context.Context) error {
	var first error
	if s.httpSrv != nil {
		if err := s.httpSrv.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	if err := s.Close(); err != nil && first == nil {
		first = err
	}
	return first
}

func (s *Server) ListenAndServe() error {
	log.Printf("Starting o11y-workbench web dashboard on %s", s.addr)
	s.httpSrv = &http.Server{Addr: s.addr, Handler: s.mux}
	err := s.httpSrv.ListenAndServe()
	// If Shutdown already ran Close, this is a no-op; otherwise reclaim workers
	// when ListenAndServe returns for any reason.
	_ = s.Close()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// ListenAndServeContext serves until ctx is cancelled, then shuts down HTTP + workers.
func (s *Server) ListenAndServeContext(ctx context.Context) error {
	log.Printf("Starting o11y-workbench web dashboard on %s", s.addr)
	s.httpSrv = &http.Server{Addr: s.addr, Handler: s.mux}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		_ = s.Close()
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-errCh
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
