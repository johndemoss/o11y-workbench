package ottlengine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// StartWorker launches an ottl worker binary that listens on 127.0.0.1:0 and
// prints a single READY line before serving. stop terminates the process with
// SIGTERM, then Kill after 2s if still running.
func StartWorker(ctx context.Context, binPath, version string) (baseURL string, stop func() error, err error) {
	if binPath == "" {
		return "", nil, fmt.Errorf("worker binary path is empty for version %s", version)
	}
	cmd := exec.CommandContext(ctx, binPath, "--listen", "127.0.0.1:0")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, fmt.Errorf("stdout pipe for worker %s: %w", version, err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("start worker %s: %w", version, err)
	}

	type readyResult struct {
		url string
		err error
	}
	ch := make(chan readyResult, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() {
			err := scanner.Err()
			if err == nil {
				err = fmt.Errorf("EOF before READY line")
			}
			ch <- readyResult{err: err}
			return
		}
		line := strings.TrimSpace(scanner.Text())
		const prefix = "READY "
		if !strings.HasPrefix(line, prefix) {
			ch <- readyResult{err: fmt.Errorf("unexpected READY line: %q", line)}
			return
		}
		ch <- readyResult{url: strings.TrimSpace(strings.TrimPrefix(line, prefix))}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
			return "", nil, fmt.Errorf("worker %s ready: %w", version, res.err)
		}
		baseURL = res.url
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return "", nil, fmt.Errorf("worker %s: timeout waiting for READY line", version)
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return "", nil, fmt.Errorf("worker %s: %w", version, ctx.Err())
	}

	var stopped bool
	stop = func() error {
		if stopped {
			return nil
		}
		stopped = true
		if cmd.Process == nil {
			return nil
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			_ = cmd.Process.Kill()
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			return ignoreExpectedStopErr(err)
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			return ignoreExpectedStopErr(<-done)
		}
	}
	return baseURL, stop, nil
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
