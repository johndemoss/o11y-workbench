package server

import "testing"

func TestNormalizeTAPFiltersAppliesDefaults(t *testing.T) {
	filters := normalizeTAPFilters(RemotetapFilters{})
	if filters.MaxSpans != DefaultMaxSpans || filters.MaxBytes != DefaultMaxBytes || filters.SamplingType != "none" {
		t.Fatalf("defaults not applied: %+v", filters)
	}
	if err := validateTAPFilters(filters); err != nil {
		t.Fatalf("defaulted filters rejected: %v", err)
	}
	negative := normalizeTAPFilters(RemotetapFilters{MaxSpans: -1})
	if err := validateTAPFilters(negative); err == nil {
		t.Fatal("negative maxSpans must stay rejected")
	}
}
