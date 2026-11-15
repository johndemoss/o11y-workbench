package engine

import "testing"

func TestImportRejectsParallelAndUnsupported(t *testing.T) {
	for _, config := range []string{
		"service: {pipelines: {traces: {}, traces/other: {}}}",
		"processors: {batch: {timeout: 1s}}\nservice: {pipelines: {traces: {processors: [batch]}}}",
	} {
		if got, err := ImportCollectorConfig(config); err == nil || got != nil {
			t.Fatalf("expected atomic error: %v, %v", got, err)
		}
	}
}
