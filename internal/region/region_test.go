package region_test

import (
	"sync"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/region"
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

func TestConcurrentEndpointAccess(t *testing.T) {
	defer region.SetEndpoints(nil)
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 1000 {
				if worker%2 == 0 {
					if i%2 == 0 {
						region.SetEndpoints([]string{"one", "two"})
					} else {
						region.SetEndpoints(nil)
					}
				} else {
					for _, name := range region.All() {
						_ = region.GetEndpoint(name)
					}
					_ = region.Assign(int64(worker), i)
				}
			}
		}()
	}
	wg.Wait()
}

func TestDefaultEndpointsAreIndependent(t *testing.T) {
	defaults := region.DefaultEndpoints()
	defaults[0].Endpoint = "changed"
	if got := region.DefaultEndpoints()[0].Endpoint; got != "localhost:4317" {
		t.Fatalf("default endpoint = %q", got)
	}
}
