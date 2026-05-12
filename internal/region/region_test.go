package region_test

import (
	"testing"

	"github.com/johndemoss/mocktracegen/internal/region"
)

func TestAll_ReturnsThree(t *testing.T) {
	all := region.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 regions, got %d", len(all))
	}
}

func TestAssign_Deterministic(t *testing.T) {
	got1 := region.Assign(42, 7)
	got2 := region.Assign(42, 7)
	if got1 != got2 {
		t.Fatalf("expected deterministic assignment: got %q, want %q", got1, got2)
	}
}

func TestAssign_AllRegionsUsed(t *testing.T) {
	use := make(map[region.Name]bool)
	for i := 0; i < 100; i++ {
		r := region.Assign(seed(42), i)
		use[r] = true
	}
	if len(use) != 3 {
		t.Fatalf("expected all 3 regions to be used, only saw %v", use)
	}
}

func seed(s int64) int64 { return s }

func TestGetEndpoint_Valid(t *testing.T) {
	for _, r := range region.All() {
		ep := region.GetEndpoint(r)
		if ep == "" {
			t.Fatalf("GetEndpoint(%q) returned empty endpoint", r)
		}
	}
}

func TestResourceAttributes_ContainsServiceName(t *testing.T) {
	attrs := region.ResourceAttributes(region.RegionLocal, "test-svc")
	for _, a := range attrs {
		if a.Key == "service.name" {
			if a.Value.GetStringValue() != "test-svc" {
				t.Fatalf("expected service.name=test-svc, got %s", a.Value.GetStringValue())
			}
			return
		}
	}
	t.Fatal("service.name attribute not found")
}

func TestSetEndpoints_CustomCount(t *testing.T) {
	endpoints := []string{"a:4317", "b:4317", "c:4317", "d:4317"}
	region.SetEndpoints(endpoints)
	defer region.SetEndpoints(nil)

	all := region.All()
	if len(all) != 4 {
		t.Fatalf("expected 4 endpoints, got %d", len(all))
	}

	use := make(map[region.Name]bool)
	for i := 0; i < 200; i++ {
		r := region.Assign(42, i)
		use[r] = true
		ep := region.GetEndpoint(r)
		if ep == "" {
			t.Fatalf("GetEndpoint(%q) returned empty", r)
		}
	}
	if len(use) != 4 {
		t.Fatalf("expected all 4 endpoints to be used, got %d", len(use))
	}
}

func TestSetEndpoints_SingleEndpoint(t *testing.T) {
	region.SetEndpoints([]string{"single:4317"})
	defer region.SetEndpoints(nil)

	all := region.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(all))
	}

	for i := 0; i < 50; i++ {
		r := region.Assign(42, i)
		if r != all[0] {
			t.Fatalf("expected all traces to map to single endpoint, got %q", r)
		}
	}

	if got := region.GetEndpoint(all[0]); got != "single:4317" {
		t.Fatalf("GetEndpoint = %q, want %q", got, "single:4317")
	}
}

func TestResourceAttributes_ContainsRegion(t *testing.T) {
	r := region.RegionUSEU
	attrs := region.ResourceAttributes(r, "test-svc")
	for _, a := range attrs {
		if a.Key == "cloud.region" {
			if a.Value.GetStringValue() != string(r) {
				t.Fatalf("expected cloud.region=%s, got %s", r, a.Value.GetStringValue())
			}
			return
		}
	}
	t.Fatalf("cloud.region attribute not found in %d attributes", len(attrs))
}
