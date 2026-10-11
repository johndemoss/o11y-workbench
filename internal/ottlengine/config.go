package ottlengine

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// parseEnvFile reads KEY=VAL lines from path. Blank lines and # comments are ignored.
func parseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return out, nil
}

// WorkerBinding maps a contrib version to a worker binary path.
type WorkerBinding struct {
	Version string
	Path    string
}

// parseWorkerBindings parses "version=path,version=path" from O11Y_OTTL_WORKERS.
func parseWorkerBindings(s string) ([]WorkerBinding, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []WorkerBinding
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		ver, path, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(ver) == "" || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("invalid O11Y_OTTL_WORKERS entry: %q", part)
		}
		out = append(out, WorkerBinding{
			Version: strings.TrimSpace(ver),
			Path:    strings.TrimSpace(path),
		})
	}
	return out, nil
}

func findWorkerBinary(version string) (string, bool) {
	name := "ottl-worker-" + version
	candidates := []string{
		filepath.Join("bin", name),
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, name),
			filepath.Join(dir, "bin", name),
		)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, true
		}
	}
	return "", false
}

// resolveWorkerBindings returns worker version→path pairs.
// Prefer O11Y_OTTL_WORKERS; otherwise use OTEL_OTTL_WORKER_VERSIONS from
// versions/otel.env (or the environment) and discover bin/ottl-worker-<ver>.
func resolveWorkerBindings() ([]WorkerBinding, error) {
	if raw := os.Getenv("O11Y_OTTL_WORKERS"); raw != "" {
		return parseWorkerBindings(raw)
	}

	versionsCSV := os.Getenv("OTEL_OTTL_WORKER_VERSIONS")
	if versionsCSV == "" {
		envPath := "versions/otel.env"
		if m, err := parseEnvFile(envPath); err == nil {
			versionsCSV = m["OTEL_OTTL_WORKER_VERSIONS"]
		}
	}
	if versionsCSV == "" {
		return nil, nil
	}

	var out []WorkerBinding
	for _, ver := range strings.Split(versionsCSV, ",") {
		ver = strings.TrimSpace(ver)
		if ver == "" {
			continue
		}
		path, ok := findWorkerBinary(ver)
		if !ok {
			log.Printf("ottlengine: worker binary not found for %s (looked for bin/ottl-worker-%s)", ver, ver)
			continue
		}
		out = append(out, WorkerBinding{Version: ver, Path: path})
	}
	return out, nil
}

// StartConfiguredWorkers starts worker processes for configured extra trains.
// Failures to start individual workers are logged and skipped; the returned map
// only contains successfully started engines. The stoppers slice has one entry
// per started worker.
func StartConfiguredWorkers(ctx context.Context) (map[string]Engine, []func() error) {
	bindings, err := resolveWorkerBindings()
	if err != nil {
		log.Printf("ottlengine: resolve workers: %v", err)
		return nil, nil
	}
	out := make(map[string]Engine)
	var stoppers []func() error
	for _, b := range bindings {
		baseURL, stop, err := StartWorker(ctx, b.Path, b.Version)
		if err != nil {
			log.Printf("ottlengine: start worker %s: %v", b.Version, err)
			continue
		}
		stable := contribToStable(b.Version)
		out[b.Version] = NewWorkerClient(b.Version, stable, baseURL)
		stoppers = append(stoppers, stop)
	}
	return out, stoppers
}

// contribToStable maps contrib 0.N → stable 1.(N-94).
func contribToStable(contrib string) string {
	var major, minor, patch int
	n, err := fmt.Sscanf(contrib, "%d.%d.%d", &major, &minor, &patch)
	if err != nil || n != 3 || major != 0 {
		return ""
	}
	return fmt.Sprintf("1.%d.%d", minor-94, patch)
}
