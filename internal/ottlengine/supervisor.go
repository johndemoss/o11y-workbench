package ottlengine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	workerReadyTimeout = 5 * time.Second
	workerStopGrace    = 2 * time.Second
)

// StartWorker launches an ottl worker binary that listens on 127.0.0.1:0 and
// prints a single READY line before serving. The worker runs with
// --parent-stdin and the parent retains the write end of its stdin pipe, so the
// worker exits when this process dies for any reason. After READY, stdout keeps
// draining to stderr so worker diagnostics can never block on a full pipe.
//
// done is closed once the process has been reaped, whether it exited on its own
// or through stop. stop closes stdin, sends SIGTERM, kills after a grace period
// and waits for the reaper; it is idempotent.
func StartWorker(ctx context.Context, binPath, version string) (baseURL string, done <-chan struct{}, stop func() error, err error) {
	if binPath == "" {
		return "", nil, nil, fmt.Errorf("worker binary path is empty for version %s", version)
	}
	cmd := exec.CommandContext(ctx, binPath, "--listen", "127.0.0.1:0", "--parent-stdin")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", nil, nil, fmt.Errorf("worker %s stdin: %w", version, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return "", nil, nil, fmt.Errorf("worker %s stdout: %w", version, err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return "", nil, nil, fmt.Errorf("start worker %s: %w", version, err)
	}

	var serving, stopping atomic.Bool
	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		_ = stdin.Close()
		if serving.Load() && !stopping.Load() {
			log.Printf("ottlengine: worker %s (pid %d) exited unexpectedly: %v; withdrawn from the registry", version, cmd.Process.Pid, waitErr)
		}
		close(exited)
	}()

	var once sync.Once
	stop = func() error {
		once.Do(func() {
			stopping.Store(true)
			select {
			case <-exited:
				return
			default:
			}
			_ = stdin.Close()
			_ = cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-exited:
			case <-time.After(workerStopGrace):
				_ = cmd.Process.Kill()
				<-exited
			}
		})
		<-exited
		return ignoreExpectedStopErr(waitErr)
	}

	type readyResult struct {
		url string
		err error
	}
	ready := make(chan readyResult, 1)
	go func() {
		reader := bufio.NewReader(stdout)
		line, err := reader.ReadSlice('\n')
		if err != nil {
			ready <- readyResult{err: fmt.Errorf("read READY line: %w", err)}
		} else {
			ready <- readyResult{url: strings.TrimSpace(string(line))}
		}
		_, _ = io.Copy(os.Stderr, reader)
	}()

	timer := time.NewTimer(workerReadyTimeout)
	defer timer.Stop()
	select {
	case res := <-ready:
		if res.err == nil {
			baseURL, res.err = parseReadyLine(res.url)
		}
		if res.err != nil {
			_ = stop()
			return "", nil, nil, fmt.Errorf("worker %s: %w", version, res.err)
		}
		serving.Store(true)
		return baseURL, exited, stop, nil
	case <-timer.C:
		_ = stop()
		return "", nil, nil, fmt.Errorf("worker %s: timeout waiting for READY", version)
	case <-ctx.Done():
		_ = stop()
		return "", nil, nil, fmt.Errorf("worker %s: %w", version, ctx.Err())
	case <-exited:
		return "", nil, nil, fmt.Errorf("worker %s exited before READY: %v", version, waitErr)
	}
}

// parseReadyLine accepts only "READY http://<loopback-ip>:<port>".
func parseReadyLine(line string) (string, error) {
	raw, ok := strings.CutPrefix(line, "READY ")
	if !ok {
		return "", fmt.Errorf("unexpected READY line: %q", line)
	}
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid READY URL %q", raw)
	}
	host, port, err := net.SplitHostPort(u.Host)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() || port == "" {
		return "", fmt.Errorf("invalid READY address %q", u.Host)
	}
	return raw, nil
}

// ignoreExpectedStopErr treats SIGTERM/Kill exits as success for supervised workers.
func ignoreExpectedStopErr(err error) error {
	if err == nil {
		return nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return nil
	}
	return err
}
