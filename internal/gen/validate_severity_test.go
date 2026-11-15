package gen

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/region"
)

func TestValidateConfigSeverityDistribution(t *testing.T) {
	tests := []struct {
		name    string
		dist    SeverityDistribution
		wantErr string
	}{
		{name: "valid weights", dist: SeverityDistribution{Info: 60, Warn: 25, Error: 12, Fatal: 3}},
		{name: "all zero uses defaults", dist: SeverityDistribution{}},
		{name: "negative info", dist: SeverityDistribution{Info: -1}, wantErr: "severity_dist.info"},
		{name: "negative warn", dist: SeverityDistribution{Warn: -1}, wantErr: "severity_dist.warn"},
		{name: "negative error", dist: SeverityDistribution{Error: -1}, wantErr: "severity_dist.error"},
		{name: "negative fatal", dist: SeverityDistribution{Fatal: -1}, wantErr: "severity_dist.fatal"},
		{name: "sum overflow", dist: SeverityDistribution{Info: math.MaxInt, Warn: 1}, wantErr: "overflow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Regions = []region.Name{region.RegionLocal}
			cfg.Signals = Signals{Traces: true}
			cfg.LogConfig.SeverityDist = tt.dist
			err := ValidateConfig(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateConfig() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateConfig() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLogGeneratorRejectsInvalidSeverity(t *testing.T) {
	cfg := DefaultLogConfig()
	cfg.Enabled = true
	cfg.LogCount = 1
	cfg.SeverityDist = SeverityDistribution{Info: -1}

	gen := NewLogGenerator(cfg)
	if err := gen.Run(context.Background(), nil); err == nil {
		t.Fatal("Run() error = nil, want invalid severity error")
	}
	refs := make(chan []SpanRef)
	close(refs)
	if err := gen.RunCorrelated(context.Background(), refs, nil); err == nil {
		t.Fatal("RunCorrelated() error = nil, want invalid severity error")
	}
}
