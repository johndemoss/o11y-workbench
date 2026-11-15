package main

import (
	"context"
	"testing"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

type shutdownProbe struct{ called bool }

func (*shutdownProbe) Export(context.Context, []*tracev1.ResourceSpans) error { return nil }
func (p *shutdownProbe) Shutdown(ctx context.Context) error {
	p.called = true
	if ctx.Err() != nil {
		return ctx.Err()
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		return context.DeadlineExceeded
	}
	return nil
}
func TestCanceledRunStillShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probe := &shutdownProbe{}
	if err := runAndShutdown(ctx, probe, func() error { return ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	if !probe.called {
		t.Fatal("shutdown was not called")
	}
}
