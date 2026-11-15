package export

import (
	"context"
	"errors"
	"testing"
	"time"

	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type timeoutProbe struct {
	started chan struct{}
	block   bool
	err     error
}

func (p *timeoutProbe) observe(ctx context.Context) error {
	if p.err != nil {
		return p.err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.started != nil {
		close(p.started)
	}
	if p.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}
func (*timeoutProbe) Shutdown(context.Context) error { return nil }

type timeoutTraces struct{ *timeoutProbe }

func (p timeoutTraces) Export(ctx context.Context, _ []*tracev1.ResourceSpans) error {
	return p.observe(ctx)
}

type timeoutLogs struct{ *timeoutProbe }

func (p timeoutLogs) Export(ctx context.Context, _ []*logsv1.ResourceLogs) error {
	return p.observe(ctx)
}

type timeoutMetrics struct{ *timeoutProbe }

func (p timeoutMetrics) Export(ctx context.Context, _ []*metricspb.ResourceMetrics) error {
	return p.observe(ctx)
}

func TestExportTimeoutOwnerLifetime(t *testing.T) {
	for name, wrap := range map[string]func(context.Context, *timeoutProbe) func(context.Context) error{
		"traces": func(owner context.Context, p *timeoutProbe) func(context.Context) error {
			return func(ctx context.Context) error {
				return WithExportTimeoutContext(owner, timeoutTraces{p}, time.Minute).Export(ctx, nil)
			}
		},
		"logs": func(owner context.Context, p *timeoutProbe) func(context.Context) error {
			return func(ctx context.Context) error {
				return WithLogExportTimeoutContext(owner, timeoutLogs{p}, time.Minute).Export(ctx, nil)
			}
		},
		"metrics": func(owner context.Context, p *timeoutProbe) func(context.Context) error {
			return func(ctx context.Context) error {
				return WithMetricExportTimeoutContext(owner, timeoutMetrics{p}, time.Minute).Export(ctx, nil)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			owner, cancelOwner := context.WithCancel(context.Background())
			defer cancelOwner()
			generation, cancelGeneration := context.WithCancel(context.Background())
			cancelGeneration()
			// Final flushing remains possible after generation/request deadlines expire.
			if err := wrap(owner, &timeoutProbe{})(generation); err != nil {
				t.Fatalf("final flush canceled with generation: %v", err)
			}
			probe := &timeoutProbe{started: make(chan struct{}), block: true}
			done := make(chan error, 1)
			go func() { done <- wrap(owner, probe)(generation) }()
			select {
			case <-probe.started:
			case <-time.After(time.Second):
				t.Fatal("export did not start")
			}
			cancelOwner()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("owner cancellation: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("owner shutdown did not cancel an in-flight export")
			}
		})
	}
}

func TestExportTimeoutOwnerErrorClassification(t *testing.T) {
	rpcErr := errors.New("rpc failure")
	for name, wrap := range map[string]func(context.Context, *timeoutProbe) error{
		"traces": func(owner context.Context, p *timeoutProbe) error {
			return WithExportTimeoutContext(owner, timeoutTraces{p}, time.Minute).Export(context.Background(), nil)
		},
		"logs": func(owner context.Context, p *timeoutProbe) error {
			return WithLogExportTimeoutContext(owner, timeoutLogs{p}, time.Minute).Export(context.Background(), nil)
		},
		"metrics": func(owner context.Context, p *timeoutProbe) error {
			return WithMetricExportTimeoutContext(owner, timeoutMetrics{p}, time.Minute).Export(context.Background(), nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			deadline, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancelDeadline()
			canceled, cancel := context.WithCancel(context.Background())
			cancel()
			for _, tt := range []struct {
				name    string
				owner   context.Context
				inner   error
				want    error
				notWant error
			}{
				{"alive", context.Background(), rpcErr, rpcErr, nil},
				{"deadline rpc failure", deadline, rpcErr, rpcErr, context.DeadlineExceeded},
				{"canceled rpc failure", canceled, rpcErr, rpcErr, context.Canceled},
				{"deadline grpc invalid argument", deadline, status.Error(codes.InvalidArgument, "invalid"), nil, context.DeadlineExceeded},
				{"deadline grpc", deadline, status.Error(codes.DeadlineExceeded, "deadline"), context.DeadlineExceeded, nil},
				{"canceled grpc", canceled, status.Error(codes.Canceled, "canceled"), context.Canceled, nil},
				{"deadline context", deadline, context.DeadlineExceeded, context.DeadlineExceeded, nil},
				{"canceled context", canceled, context.Canceled, context.Canceled, nil},
			} {
				t.Run(tt.name, func(t *testing.T) {
					err := wrap(tt.owner, &timeoutProbe{err: tt.inner})
					want := tt.want
					if want == nil {
						want = tt.inner
					}
					if !errors.Is(err, want) {
						t.Fatalf("error = %v, want %v", err, want)
					}
					if tt.notWant != nil && errors.Is(err, tt.notWant) {
						t.Fatalf("error = %v, unexpectedly matches %v", err, tt.notWant)
					}
				})
			}
		})
	}
}
