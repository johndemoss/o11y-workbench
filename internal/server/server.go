package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

)

//go:embed web
var webFS embed.FS

//go:embed pipeline-ui/dist
var pipelineFS embed.FS

type Server struct {
	store           *RunStore
	mux             *http.ServeMux
	addr            string
	pipelineEnabled bool
	remotetap       *RemotetapManager

	evaluation     *evaluation.Coordinator
	evalStore      *evaluation.Store
	evalInventory  *evaluation.BuildInventory
	evalLibrary    []evaluation.LibraryEntry
	legacyCapacity bool
}

type serverConfig struct {
	addr            string
	pipelineEnabled bool
	legacyCapacity  *bool
}

func New(addr string, pipelineEnabled bool) (*Server, error) {
	return newServer(serverConfig{addr: addr, pipelineEnabled: pipelineEnabled})
}

func newServer(cfg serverConfig) (*Server, error) {
	store := NewRunStore()
	capacityStore, err := capacitystore.New(os.Getenv("O11Y_WORKBENCH_CAPACITY_MODELS_DIR"))
	if err != nil {
		return nil, fmt.Errorf("capacity store: %w", err)
	}

	evalRoot := cfg.evalRoot
	if strings.TrimSpace(evalRoot) == "" {
		evalRoot = defaultEvaluationRoot()
	}
	evalStore, err := evaluation.OpenStore(evalRoot)
	if err != nil {
		return nil, fmt.Errorf("evaluation store: %w", err)
	}

	inv := cfg.inventory
	if inv == nil {
		inv, err = evaluation.NewBuildInventory()
		if err != nil {
			return nil, fmt.Errorf("evaluation inventory: %w", err)
		}
	}

	runner := cfg.runner
	if runner == nil {
		runner = evaluation.FailClosedRunner{}
	}

	lib := cfg.library
	if lib == nil {
		lib, err = evaluation.BuiltinLibrary()
		if err != nil {
			return nil, fmt.Errorf("evaluation library: %w", err)
		}
	}

	coord, err := evaluation.NewCoordinator(evaluation.CoordinatorOptions{
		Store:     evalStore,
		Inventory: inv,
		Runner:    runner,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluation coordinator: %w", err)
	}

	legacy := legacyCapacityEnabledFromEnv()
	if cfg.legacyCapacity != nil {
		legacy = *cfg.legacyCapacity
	}

	s := &Server{
		store:           store,
		mux:             http.NewServeMux(),
		addr:            cfg.addr,
		pipelineEnabled: cfg.pipelineEnabled,
		remotetap:       NewRemotetapManager(store),
		legacyCapacity:  legacy,
	}
	if err := s.registerRoutes(); err != nil {
		coord.Close()
		return nil, fmt.Errorf("register routes: %w", err)
	}
	return s, nil
}

// Close releases evaluation coordinator resources.
func (s *Server) Close() {
	if s == nil || s.evaluation == nil {
		return
	}
	s.evaluation.Close()
}

func (s *Server) registerRoutes() error {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
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

func (s *Server) ListenAndServe() error {
	log.Printf("Starting o11y-workbench web dashboard on %s", s.addr)
	return http.ListenAndServe(s.addr, s.mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
