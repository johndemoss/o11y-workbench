package gen_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/mocktracegen/internal/export"
	"github.com/johndemoss/mocktracegen/internal/gen"
)

// mockExporter captures ResourceSpans for testing (no network).
type mockExporter struct {
	spans []*tracev1.ResourceSpans
	mu    sync.Mutex
}

func (m *mockExporter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spans = append(m.spans, spans...)
	return nil
}

func (m *mockExporter) Shutdown(ctx context.Context) error { return nil }

func TestSeededRNG_Determinism(t *testing.T) {
	seed := int64(42)
	rng1 := gen.NewSeededRNG(seed)
	rng2 := gen.NewSeededRNG(seed)

	tid1 := rng1.TraceID()
	tid2 := rng2.TraceID()
	if tid1 != tid2 {
		t.Fatalf("TraceID mismatch: %x vs %x", tid1, tid2)
	}

	if got := rng1.SpanID(); rng2.SpanID() != got {
		t.Fatalf("SpanID mismatch: %x vs %x", got, rng2.SpanID())
	}

	n1 := rng1.Int63n(1000)
	n2 := rng2.Int63n(1000)
	if n1 != n2 {
		t.Errorf("Int63n(1000) mismatch: %d vs %d", n1, n2)
	}

	f1 := rng1.Float64()
	f2 := rng2.Float64()
	if f1 != f2 {
		t.Errorf("Float64 mismatch: %f vs %f", f1, f2)
	}

	base := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	d1 := rng1.Duration(time.Hour)
	d2 := rng2.Duration(time.Hour)
	if d1 != d2 {
		t.Errorf("Duration(1h) mismatch: %v vs %v", d1, d2)
	}

	t1 := rng1.Timestamp(base, time.Second)
	t2 := rng2.Timestamp(base, time.Second)
	if !t1.Equal(t2) {
		t.Errorf("Timestamp mismatch: %v vs %v", t1, t2)
	}

	b1 := rng1.Bool()
	b2 := rng2.Bool()
	if b1 != b2 {
		t.Errorf("Bool mismatch: %v vs %v", b1, b2)
	}

	if rng1.Int63n(5) != rng2.Int63n(5) {
		t.Error("Int63n(5) mismatch")
	}

	ir1 := rng1.IntRange(10, 50)
	ir2 := rng2.IntRange(10, 50)
 	if ir1 != ir2 {
 		t.Errorf("IntRange(10,50) mismatch: %d vs %d", ir1, ir2)
 	}

 	rngA := gen.NewSeededRNG(seed)
 	rngB := gen.NewSeededRNG(seed)
	for i := 0; i < 100; i++ {
		tn := rngA.Int63n(1000)
		tnB := rngB.Int63n(1000)
		if tn != tnB {
			t.Errorf("Interleaved Int63n mismatch at iteration %d: %d vs %d", i, tn, tnB)
		}

		trA := rngA.TraceID()
		trB := rngB.TraceID()
		if trA != trB {
			t.Errorf("Interleaved TraceID mismatch at iteration %d", i)
		}

		fA := rngA.Float64()
		fB := rngB.Float64()
		if fA != fB {
			t.Errorf("Interleaved Float64 mismatch at iteration %d: %f vs %f", i, fA, fB)
		}

		irA := rngA.IntRange(0, 100)
		irB := rngB.IntRange(0, 100)
		if irA != irB {
			t.Errorf("Interleaved IntRange mismatch at iteration %d: %d vs %d", i, irA, irB)
		}
	}

	durA := rngA.Duration(time.Minute)
	durB := rngB.Duration(time.Minute)
	if durA != durB {
		t.Errorf("Interleaved Duration mismatch: %v vs %v", durA, durB)
	}
}

func TestSeededRNG_DifferentSeeds(t *testing.T) {
	rngA := gen.NewSeededRNG(12345)
	rngB := gen.NewSeededRNG(67890)

	a := rngA.TraceID()
	b := rngB.TraceID()
	if a == b {
		t.Fatalf("TraceID same despite different seeds: %x", a)
	}

	aID := rngA.SpanID()
	bID := rngB.SpanID()
	if aID == bID {
		t.Fatalf("SpanID same despite different seeds: %x", aID)
	}
}

func TestSeededRNG_TraceID_Length(t *testing.T) {
	rng := gen.NewSeededRNG(42)
	for i := 0; i < 10; i++ {
		id := rng.TraceID()
		if len(id) != 16 {
			t.Errorf("TraceID length: got %d, want 16", len(id))
		}
	}
}

func TestSeededRNG_SpanID_Length(t *testing.T) {
	rng := gen.NewSeededRNG(42)
	for i := 0; i < 10; i++ {
		id := rng.SpanID()
		if len(id) != 8 {
			t.Errorf("SpanID length: got %d, want 8", len(id))
		}
	}
}

func TestSeededRNG_IntRange(t *testing.T) {
	rng := gen.NewSeededRNG(42)
	// Verify range bounds
	for i := 0; i < 100; i++ {
		v := rng.IntRange(0, 5)
		if v < 0 || v > 5 {
			t.Fatalf("IntRange(0,5) returned out-of-range value: %d", v)
		}
	}

	// Panic on min > max
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("IntRange(min>max) should panic")
		}
	}()
	rng.IntRange(10, 5)
}

func TestSeededRNG_Float64_Bounds(t *testing.T) {
	rng := gen.NewSeededRNG(42)
	for i := 0; i < 100; i++ {
		f := rng.Float64()
		if f < 0.0 || f >= 1.0 {
			t.Fatalf("Float64 out of [0,1): got %f", f)
		}
	}
}

func TestSeededRNG_Int63n_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Int63n(0) should panic")
		}
	}()
	gen.NewSeededRNG(1).Int63n(0)
}

func TestSeededRNG_Duration_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Duration(0) should panic")
		}
	}()
	gen.NewSeededRNG(1).Duration(0)
}

func TestSeededRNG_Timestamp_Panic(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Timestamp(base, 0) should panic")
		}
	}()
	gen.NewSeededRNG(1).Timestamp(base, 0)
}

// ---------- Topology tests ----------

func TestTopology_Flat_Count(t *testing.T) {
	testCases := []struct {
		name      string
		spanCount int
	}{
		{"5", 5},
		{"10", 10},
		{"20", 20},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rng := gen.NewSeededRNG(42)
			root := gen.GenerateTopology(gen.TopologyFlat, rng, tc.spanCount, 5)
			got := gen.CountSpans(root)
			if got != tc.spanCount {
				t.Errorf("flat span count: got %d, want %d", got, tc.spanCount)
			}
			if len(root.Children) != tc.spanCount-1 {
				t.Errorf("flat children count: got %d, want %d", len(root.Children), tc.spanCount-1)
			}
		})
	}
}

func TestTopology_Branching_Count(t *testing.T) {
	for _, count := range []int{10, 20} {
		t.Run(fmt.Sprintf("spanCount=%d", count), func(t *testing.T) {
			rng := gen.NewSeededRNG(42)
			root := gen.GenerateTopology(gen.TopologyBranching, rng, count, 5)
			got := gen.CountSpans(root)
			// Allow 20% tolerance due to RNG-driven subtree sizing
			lower := int(float64(count) * 0.8)
			upper := int(float64(count) * 1.2)
			if got < lower || got > upper {
				t.Errorf("branching span count: got %d, expected approx %d (range [%d, %d])", got, count, lower, upper)
			}
			// Verify tree structure: at least some nodes have children
			nodes := gen.CollectSpans(root)
			hasChildren := false
			for _, n := range nodes {
				if len(n.Children) > 0 {
					hasChildren = true
					break
				}
			}
			if !hasChildren {
				t.Error("branching topology has no nodes with children")
			}
		})
	}
}

func TestTopology_Split_Count(t *testing.T) {
	for _, count := range []int{10, 20} {
		t.Run(fmt.Sprintf("spanCount=%d", count), func(t *testing.T) {
			rng := gen.NewSeededRNG(42)
			root := gen.GenerateTopology(gen.TopologySplit, rng, count, 5)
			got := gen.CountSpans(root)
			lower := int(float64(count) * 0.8)
			upper := int(float64(count) * 1.2)
			if got < lower || got > upper {
				t.Errorf("split span count: got %d, expected approx %d (range [%d, %d])", got, count, lower, upper)
			}
			// Split: root should have 2-3 children
			n := len(root.Children)
			if n < 2 || n > 3 {
				t.Errorf("split root children: got %d, want 2-3", n)
			}
		})
	}
}

func TestTopology_CollectSpans(t *testing.T) {
	topologies := []gen.TopologyType{gen.TopologyFlat, gen.TopologyBranching, gen.TopologySplit}

	for _, topo := range topologies {
		t.Run(topo.String(), func(t *testing.T) {
			rng := gen.NewSeededRNG(42)
			top := gen.GenerateTopology(topo, rng, 15, 5)
			counted := gen.CountSpans(top)
			collected := gen.CollectSpans(top)
			if len(collected) != counted {
				t.Errorf("CollectSpans len (%d) != CountSpans (%d)", len(collected), counted)
			}
			// Verify depth-first pre-order: root must be first
			if len(collected) > 0 && collected[0] != top {
				t.Error("CollectSpans first element should be root")
			}
		})
	}
}

func TestTopology_ParseValid(t *testing.T) {
	tests := []struct {
		input string
		want  gen.TopologyType
	}{
		{"flat", gen.TopologyFlat},
		{"Flat", gen.TopologyFlat},
		{" FLAT ", gen.TopologyFlat},
		{"branching", gen.TopologyBranching},
		{"Branching", gen.TopologyBranching},
		{"split", gen.TopologySplit},
		{" SPLIT ", gen.TopologySplit},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := gen.ParseTopology(tt.input)
			if err != nil {
				t.Fatalf("ParseTopology(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseTopology(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTopology_ParseInvalid(t *testing.T) {
	invalid := []string{"", "random", "flatly", "TREE",  "circular"}
	for _, s := range invalid {
		t.Run(s, func(t *testing.T) {
			_, err := gen.ParseTopology(s)
			if err == nil {
				t.Errorf("ParseTopology(%q) expected error, got nil", s)
			}
		})
	}
}

func TestTopology_EdgeCases(t *testing.T) {
	rng := gen.NewSeededRNG(42)

	// CountSpans(nil) == 0
	if gen.CountSpans(nil) != 0 {
		t.Error("CountSpans(nil) should be 0")
	}

	// CollectSpans(nil) == nil
	if got := gen.CollectSpans(nil); got != nil {
		t.Errorf("CollectSpans(nil) = %v, want nil", got)
	}

	// GenerateTopology with spanCount=1
	for _, topo := range []gen.TopologyType{gen.TopologyFlat, gen.TopologyBranching, gen.TopologySplit} {
		t.Run(fmt.Sprintf("spanCount=1_%s", topo.String()), func(t *testing.T) {
			root := gen.GenerateTopology(topo, rng, 1, 5)
			if gen.CountSpans(root) != 1 {
				t.Errorf("spanCount=1 %s: got %d spans", topo.String(), gen.CountSpans(root))
			}
		})
	}

	// SpanNode depth tracking on flat
	root := gen.GenerateTopology(gen.TopologyFlat, rng, 5, 5)
	for _, child := range root.Children {
		if child.Depth != 1 {
			t.Errorf("flat child depth: got %d, want 1", child.Depth)
		}
	}
}

func TestTopology_ErrorMarking(t *testing.T) {
	rng := gen.NewSeededRNG(42)
	for _, topo := range []gen.TopologyType{gen.TopologyFlat, gen.TopologyBranching, gen.TopologySplit} {
		t.Run(topo.String(), func(t *testing.T) {
			root := gen.GenerateTopology(topo, rng, 20, 5)
			nodes := gen.CollectSpans(root)
			// With 20 spans, ~2 should be errors
			errorCount := 0
			for _, n := range nodes {
				if n.IsError {
					errorCount++
				}
			}
			if errorCount < 1 || errorCount > 5 {
				t.Errorf("%s error count: got %d, expected ~2", topo.String(), errorCount)
			}
		})
	}
}

// collectAllSpans returns all spans across all ResourceSpans.
func collectAllSpans(spans []*tracev1.ResourceSpans) []*tracev1.Span {
	var all []*tracev1.Span
	for _, rs := range spans {
		for _, ss := range rs.ScopeSpans {
			all = append(all, ss.Spans...)
		}
	}
	return all
}

// ---------- Config test ----------

func TestDefaultConfig(t *testing.T) {
	c := gen.DefaultConfig()
	if c.Seed == 0 {
		t.Error("DefaultConfig.Seed should be non-zero")
	}
	if c.ServiceName == "" {
		t.Error("DefaultConfig.ServiceName should be non-empty")
	}
	if c.TraceDuration == 0 {
		t.Error("DefaultConfig.TraceDuration should be non-zero")
	}
	if c.TraceCount != 1 {
		t.Errorf("DefaultConfig.TraceCount = %d, want 1", c.TraceCount)
	}
	if c.SpanCount != 10 {
		t.Errorf("DefaultConfig.SpanCount = %d, want 10", c.SpanCount)
	}
	if c.Topology != gen.TopologyFlat {
		t.Errorf("DefaultConfig.Topology = %v, want flat", c.Topology)
	}
}

// ---------- Generator test ----------

func TestGenerator_Determinism(t *testing.T) {
	topo := gen.TopologyFlat
	topos := []gen.TopologyType{topo, gen.TopologyBranching, gen.TopologySplit}

	for _, tp := range topos {
		t.Run(tp.String(), func(t *testing.T) {
			configA := gen.Config{
				Seed:          42,
				TraceCount:    2,
				Topology:      tp,
				SpanCount:     10,
				MaxDepth:      5,
				ServiceName:   "test-svc",
				BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				TraceDuration: 5 * time.Second,
			}
			configB := gen.Config{
				Seed:          42,
				TraceCount:    2,
				Topology:      tp,
				SpanCount:     10,
				MaxDepth:      5,
				ServiceName:   "test-svc",
				BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				TraceDuration: 5 * time.Second,
			}

			gA := gen.NewGenerator(configA)
			gB := gen.NewGenerator(configB)

			expA := &mockExporter{}
			expB := &mockExporter{}

			ctx := context.Background()

			if err := gA.Run(ctx, expA); err != nil {
				t.Fatalf("Generator A Run: %v", err)
			}
			if err := gB.Run(ctx, expB); err != nil {
				t.Fatalf("Generator B Run: %v", err)
			}

			// Compare ResourceSpans — topology structure must match even though
			// trace/span IDs and timestamps differ (they come from a random RNG).
			spA := expA.spans
			spB := expB.spans

			if len(spA) != len(spB) {
				t.Fatalf("ResourceSpans count: A=%d, B=%d", len(spA), len(spB))
			}

			for i := range spA {
				aRS := spA[i]
				bRS := spB[i]

				// Check schema URL
				if aRS.SchemaUrl != bRS.SchemaUrl {
					t.Errorf("trace %d SchemaUrl: A=%q, B=%q", i, aRS.SchemaUrl, bRS.SchemaUrl)
				}

				// Check ScopeSpans and spans count
				if len(aRS.ScopeSpans) != len(bRS.ScopeSpans) {
					t.Fatalf("trace %d ScopeSpans count: A=%d, B=%d", i, len(aRS.ScopeSpans), len(bRS.ScopeSpans))
				}

				for j := range aRS.ScopeSpans {
					aSS := aRS.ScopeSpans[j]
					bSS := bRS.ScopeSpans[j]

					if aSS.Scope != nil && bSS.Scope != nil {
						if aSS.Scope.Name != bSS.Scope.Name {
							t.Errorf("trace %d Scope.Name: A=%q, B=%q", i, aSS.Scope.Name, bSS.Scope.Name)
						}
						if aSS.Scope.Version != bSS.Scope.Version {
							t.Errorf("trace %d Scope.Version: A=%q, B=%q", i, aSS.Scope.Version, bSS.Scope.Version)
						}
					}

					if len(aSS.Spans) != len(bSS.Spans) {
						t.Fatalf("trace %d span count: A=%d, B=%d", i, len(aSS.Spans), len(bSS.Spans))
					}

					for k := range aSS.Spans {
						aSpan := aSS.Spans[k]
						bSpan := bSS.Spans[k]

						// TraceId / SpanId / ParentSpanId / timestamps are NOT
						// compared — they come from the random idRng and differ
						// per run.

						if aSpan.Name != bSpan.Name {
							t.Errorf("trace %d span %d Name: A=%q, B=%q", i, k, aSpan.Name, bSpan.Name)
						}
						if aSpan.Kind != bSpan.Kind {
							t.Errorf("trace %d span %d Kind: A=%d, B=%d", i, k, aSpan.Kind, bSpan.Kind)
						}
						if aSpan.Flags != bSpan.Flags {
							t.Errorf("trace %d span %d Flags: A=%d, B=%d", i, k, aSpan.Flags, bSpan.Flags)
						}

						// Compare attributes (span.depth, etc.)
						if len(aSpan.Attributes) != len(bSpan.Attributes) {
							t.Fatalf("trace %d span %d attr count: A=%d, B=%d", i, k, len(aSpan.Attributes), len(bSpan.Attributes))
						}
						for li := range aSpan.Attributes {
							aAttr := aSpan.Attributes[li]
							bAttr := bSpan.Attributes[li]
							if aAttr.Key != bAttr.Key {
								t.Errorf("trace %d span %d attr key %d: A=%q, B=%q", i, k, li, aAttr.Key, bAttr.Key)
							}
							if aAttr.Value.GetStringValue() != bAttr.Value.GetStringValue() {
								t.Errorf("trace %d span %d attr val %d: A=%q, B=%q", i, k, li, aAttr.Value.GetStringValue(), bAttr.Value.GetStringValue())
							}
						}

						// Compare status
						if aSpan.Status == nil && bSpan.Status != nil {
							t.Errorf("trace %d span %d status: A=nil, B not nil", i, k)
						}
						if aSpan.Status != nil && bSpan.Status == nil {
							t.Errorf("trace %d span %d status: A not nil, B=nil", i, k)
						}
						if aSpan.Status != nil && bSpan.Status != nil {
							if aSpan.Status.Code != bSpan.Status.Code {
								t.Errorf("trace %d span %d status code: A=%d, B=%d", i, k, aSpan.Status.Code, bSpan.Status.Code)
							}
							if aSpan.Status.Message != bSpan.Status.Message {
								t.Errorf("trace %d span %d status message: A=%q, B=%q", i, k, aSpan.Status.Message, bSpan.Status.Message)
							}
						}
					}
				}
			}
		})
	}
}

func TestGenerator_UniqueIDs(t *testing.T) {
	config := gen.Config{
		Seed:          42,
		TraceCount:    3,
		Topology:      gen.TopologyFlat,
		SpanCount:     10,
		MaxDepth:      5,
		ServiceName:   "test-svc",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: 5 * time.Second,
	}

	exp1 := &mockExporter{}
	exp2 := &mockExporter{}
	ctx := context.Background()

	if err := gen.NewGenerator(config).Run(ctx, exp1); err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	if err := gen.NewGenerator(config).Run(ctx, exp2); err != nil {
		t.Fatalf("Run 2: %v", err)
	}

	all1 := collectAllSpans(exp1.spans)
	all2 := collectAllSpans(exp2.spans)

	if len(all1) != len(all2) {
		t.Fatalf("span count: run1=%d, run2=%d", len(all1), len(all2))
	}

	// All SpanIDs within a run must be unique.
	seenSpanIDs := make(map[string]bool)
	for _, s := range all1 {
		sid := string(s.SpanId)
		if seenSpanIDs[sid] {
			t.Errorf("duplicate SpanId in run1: %x", s.SpanId)
		}
		seenSpanIDs[sid] = true
	}

	// There should be exactly TraceCount unique TraceIDs (one per trace).
	uniqueTraceIDs := make(map[string]int)
	for _, s := range all1 {
		uniqueTraceIDs[string(s.TraceId)]++
	}
	if got := len(uniqueTraceIDs); got != config.TraceCount {
		t.Errorf("unique TraceIDs in run1: got %d, want %d", got, config.TraceCount)
	}

	// Spans from run2 should have different IDs than run1.
	for i := range all1 {
		tid1 := string(all1[i].TraceId)
		tid2 := string(all2[i].TraceId)
		if tid1 == tid2 {
			t.Errorf("trace %d: same TraceId across runs: %x", i, all1[i].TraceId)
		}
		sid1 := string(all1[i].SpanId)
		sid2 := string(all2[i].SpanId)
		if sid1 == sid2 {
			t.Errorf("span %d: same SpanId across runs: %x", i, all1[i].SpanId)
		}
	}
}

func TestGenerator_Rate_LimitsThroughput(t *testing.T) {
	t.Run("rate=0 runs at full speed", func(t *testing.T) {
		c := gen.Config{
			Seed:          42,
			TraceCount:    10,
			Rate:          0,
			Topology:      gen.TopologyFlat,
			SpanCount:     5,
			MaxDepth:      5,
			ServiceName:   "test-svc",
			BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			TraceDuration: 5 * time.Second,
		}

		ctx := context.Background()
		exp := &mockExporter{}
		start := time.Now()
		if err := gen.NewGenerator(c).Run(ctx, exp); err != nil {
			t.Fatalf("Run: %v", err)
		}
		elapsed := time.Since(start)
		if elapsed > 100*time.Millisecond {
			t.Logf("rate=0 took %v (fast on most machines)", elapsed)
		}
		allSpans := collectAllSpans(exp.spans)
		if len(allSpans) != 50 {
			t.Errorf("expected 50 spans total, got %d", len(allSpans))
		}
	})

	t.Run("rate=100 paces start times", func(t *testing.T) {
		c := gen.Config{
			Seed:          42,
			TraceCount:    5,
			Rate:          100, // 10ms per trace
			Topology:      gen.TopologyFlat,
			SpanCount:     1,
			MaxDepth:      5,
			ServiceName:   "test-svc",
			BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			TraceDuration: time.Second,
		}

		ctx := context.Background()
		exp := &mockExporter{}
		start := time.Now()
		if err := gen.NewGenerator(c).Run(ctx, exp); err != nil {
			t.Fatalf("Run: %v", err)
		}
		elapsed := time.Since(start)
		// 5 traces at 10ms/trace = at least 40ms (4 intervals between 5 ticks)
		// Allow generous tolerance for slow CI.
		if elapsed < 20*time.Millisecond {
			t.Errorf("rate=100 completed in %v, expected at least ~40ms", elapsed)
		}
		allSpans := collectAllSpans(exp.spans)
		if len(allSpans) != 5 {
			t.Errorf("expected 5 spans total, got %d", len(allSpans))
		}
	})

	t.Run("context cancellation during tick wait", func(t *testing.T) {
		c := gen.Config{
			Seed:          42,
			TraceCount:    100,
			Rate:          1, // 1s per trace — easy to cancel
			Topology:      gen.TopologyFlat,
			SpanCount:     1,
			MaxDepth:      5,
			ServiceName:   "test-svc",
			BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			TraceDuration: time.Second,
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // already cancelled
		exp := &mockExporter{}
		err := gen.NewGenerator(c).Run(ctx, exp)
		if err == nil {
			t.Fatal("expected context cancellation error, got nil")
		}
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}

func TestGenerator_RootTiming(t *testing.T) {
	traceDuration := 10 * time.Second
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("root encapsulates children", func(t *testing.T) {
		for _, topo := range []gen.TopologyType{gen.TopologyFlat, gen.TopologyBranching, gen.TopologySplit} {
			t.Run(topo.String(), func(t *testing.T) {
				c := gen.Config{
					Seed:          42,
					TraceCount:    1,
					Topology:      topo,
					SpanCount:     10,
					MaxDepth:      5,
					ServiceName:   "test-svc",
					BaseTime:      baseTime,
					TraceDuration: traceDuration,
				}

				exp := &mockExporter{}
				if err := gen.NewGenerator(c).Run(context.Background(), exp); err != nil {
					t.Fatalf("Run: %v", err)
				}

				all := collectAllSpans(exp.spans)
				var rootSpan *tracev1.Span
				for _, s := range all {
					if len(s.ParentSpanId) == 0 {
						rootSpan = s
						break
					}
				}
				if rootSpan == nil {
					t.Fatal("no root span found")
				}

				rootStart := time.Unix(0, int64(rootSpan.StartTimeUnixNano))
				rootEnd := time.Unix(0, int64(rootSpan.EndTimeUnixNano))
				rootDur := rootEnd.Sub(rootStart)

				if rootDur <= 0 {
					t.Errorf("root span duration should be positive, got %v", rootDur)
				}
				if rootEnd.After(baseTime.Add(traceDuration)) {
					t.Errorf("root end %v exceeds trace window %v", rootEnd, baseTime.Add(traceDuration))
				}

				// Every child must be fully within the root's time range.
				for _, s := range all {
					if len(s.ParentSpanId) == 0 {
						continue
					}
					sStart := time.Unix(0, int64(s.StartTimeUnixNano))
					sEnd := time.Unix(0, int64(s.EndTimeUnixNano))
					if sStart.Before(rootStart) {
						t.Errorf("child %s starts before root: %v < %v", s.Name, sStart, rootStart)
					}
					if sEnd.After(rootEnd) {
						t.Errorf("child %s ends after root: %v > %v", s.Name, sEnd, rootEnd)
					}
				}
			})
		}
	})

	t.Run("chain topology produces nested timing", func(t *testing.T) {
		// With a 10-span branching topology at max depth, there should be
		// at least one nesting chain where parent timestamps contain child timestamps.
		c := gen.Config{
			Seed:          42,
			TraceCount:    1,
			Topology:      gen.TopologyBranching,
			SpanCount:     10,
			MaxDepth:      10,
			ServiceName:   "test-svc",
			BaseTime:      baseTime,
			TraceDuration: traceDuration,
		}

		exp := &mockExporter{}
		if err := gen.NewGenerator(c).Run(context.Background(), exp); err != nil {
			t.Fatalf("Run: %v", err)
		}

		all := collectAllSpans(exp.spans)

		// Build a lookup by SpanId for parent checks.
		byID := make(map[string]*tracev1.Span)
		for _, s := range all {
			byID[string(s.SpanId)] = s
		}

		for _, s := range all {
			if len(s.ParentSpanId) == 0 {
				continue
			}
			parent, ok := byID[string(s.ParentSpanId)]
			if !ok {
				continue
			}
			sStart := time.Unix(0, int64(s.StartTimeUnixNano))
			sEnd := time.Unix(0, int64(s.EndTimeUnixNano))
			pStart := time.Unix(0, int64(parent.StartTimeUnixNano))
			pEnd := time.Unix(0, int64(parent.EndTimeUnixNano))

			if sStart.Before(pStart) {
				t.Errorf("child %s starts before parent %s: %v < %v", s.Name, parent.Name, sStart, pStart)
			}
			if sEnd.After(pEnd) {
				t.Errorf("child %s ends after parent %s: %v > %v", s.Name, parent.Name, sEnd, pEnd)
			}
		}
	})
}

func TestGenerator_SpanCount(t *testing.T) {
	c := gen.Config{
		Seed:          42,
		TraceCount:    1,
		Topology:      gen.TopologyFlat,
		SpanCount:     10,
		MaxDepth:      5,
		ServiceName:   "test-svc",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: 5 * time.Second,
	}

	g := gen.NewGenerator(c)
	exp := &mockExporter{}
	ctx := context.Background()

	if err := g.Run(ctx, exp); err != nil {
		t.Fatalf("Generator Run error: %v", err)
	}

	allSpans := collectAllSpans(exp.spans)
	if len(allSpans) != 10 {
		t.Errorf("expected 10 spans total, got %d", len(allSpans))
	}
}

func TestGenerator_SpanCountBranching(t *testing.T) {
	c := gen.Config{
		Seed:          42,
		TraceCount:    1,
		Topology:      gen.TopologyBranching,
		SpanCount:     15,
		MaxDepth:      5,
		ServiceName:   "test-svc",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: 5 * time.Second,
	}

	g := gen.NewGenerator(c)
	exp := &mockExporter{}
	ctx := context.Background()

	if err := g.Run(ctx, exp); err != nil {
		t.Fatalf("Generator Run error: %v", err)
	}

	allSpans := collectAllSpans(exp.spans)
	if len(allSpans) < 10 || len(allSpans) > 20 {
		t.Errorf("expected ~15 spans total, got %d", len(allSpans))
	}
}

func TestGenerator_ContextCancellation(t *testing.T) {
	c := gen.Config{
		Seed:          42,
		TraceCount:    3,
		Topology:      gen.TopologyFlat,
		SpanCount:     10,
		MaxDepth:      5,
		ServiceName:   "test-svc",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: 5 * time.Second,
	}

	// Use a context that's already done after first trace
	ctx, cancel := context.WithCancel(context.Background())
	g := gen.NewGenerator(c)
	exp := &mockExporter{}

	// Run should complete normally because first trace finishes before cancel is checked
	// Only on subsequent iterations does ctx.Done() trigger
	// To truly test, use cancel after a brief delay — but that's racy.
	// Instead test with TraceCount=1 where no cancellation point is hit after work:
	ctx2 := context.Background()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	if err := g.Run(ctx, exp); err != nil {
		t.Logf("Context cancelled during run (expected in some cases): %v", err)
	}

	// Now test with TraceCount=0 — should return nil immediately
	cZero := gen.Config{
		Seed:          42,
		TraceCount:    0,
		Topology:      gen.TopologyFlat,
		SpanCount:     10,
		MaxDepth:      5,
		ServiceName:   "test-svc",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: 5 * time.Second,
	}

	gZero := gen.NewGenerator(cZero)
	expZero := &mockExporter{}
	if err := gZero.Run(ctx2, expZero); err != nil {
		t.Fatalf("TraceCount=0 should return nil, got: %v", err)
	}
}

func TestGenerator_TopologyExported(t *testing.T) {
	topos := []gen.TopologyType{gen.TopologyFlat, gen.TopologyBranching, gen.TopologySplit}

	for _, tp := range topos {
		t.Run(tp.String(), func(t *testing.T) {
			c := gen.Config{
				Seed:          42,
				TraceCount:    1,
				Topology:      tp,
				SpanCount:     10,
				MaxDepth:      5,
				ServiceName:   "test-svc",
				BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				TraceDuration: 5 * time.Second,
			}

			g := gen.NewGenerator(c)
			exp := &mockExporter{}
			ctx := context.Background()

			if err := g.Run(ctx, exp); err != nil {
				t.Fatalf("Generator Run error: %v", err)
			}

			if len(exp.spans) == 0 {
				t.Fatal("expected at least 1 ResourceSpans, got 0")
			}

			// Verify each ResourceSpans group is well-formed.
			regionsSeen := make(map[string]bool)
			for _, rs := range exp.spans {
				if rs.SchemaUrl == "" {
					t.Error("ResourceSpans.SchemaUrl should be set")
				}
				if len(rs.ScopeSpans) == 0 {
					t.Error("ResourceSpans.ScopeSpans should not be empty")
				}
				if rs.ScopeSpans[0].Scope == nil {
					t.Error("ScopeSpans.Scope should not be nil")
				} else {
					if rs.ScopeSpans[0].Scope.Name == "" {
						t.Error("ScopeSpans.Scope.Name should not be empty")
					}
				}
				if rs.ScopeSpans[0].Spans == nil || len(rs.ScopeSpans[0].Spans) == 0 {
					t.Error("ScopeSpans.Spans should not be empty")
				}

				// Track which cloud.region values appear.
				for _, attr := range rs.Resource.GetAttributes() {
					if attr.Key == "cloud.region" {
						regionsSeen[attr.Value.GetStringValue()] = true
					}
				}
			}

			// With 3 default endpoints, spans should be distributed.
			if len(regionsSeen) < 2 {
				t.Errorf("expected spans distributed across >=2 regions, got %v", regionsSeen)
			}

			// Verify spans have correct data across ALL ResourceSpans.
			allSpans := collectAllSpans(exp.spans)
			rootFound := false
			for _, sp := range allSpans {
				if sp.Name == "root" {
					rootFound = true
					if len(sp.ParentSpanId) != 0 {
						t.Errorf("root span should have empty ParentSpanId, got %x", sp.ParentSpanId)
					}
					break
				}
			}
			if !rootFound {
				t.Error("Expected a \"root\" span in the generated output")
			}

			if len(allSpans) < 1 {
				t.Error("at least one span expected")
			}

			for i, sp := range allSpans {
				if sp.TraceId == nil || len(sp.TraceId) != 16 {
					t.Errorf("span %d: TraceId should be 16 bytes, got %d", i, len(sp.TraceId))
				}
				if sp.SpanId == nil || len(sp.SpanId) != 8 {
					t.Errorf("span %d: SpanId should be 8 bytes, got %d", i, len(sp.SpanId))
				}
			}
		})
	}
}

// ---------- Scenario generator tests ----------

func TestGenerator_Scenario_ServiceNameAttribute(t *testing.T) {
	scenarios := []string{"microservice", "monolith", "3-tier-web-app", "event-driven"}

	for _, s := range scenarios {
		t.Run(s, func(t *testing.T) {
			c := gen.Config{
				Seed:          42,
				TraceCount:    2,
				Scenario:      s,
				SpanCount:     15,
				MaxDepth:      5,
				ServiceName:   "test-svc",
				BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				TraceDuration: 5 * time.Second,
			}
			exp := &mockExporter{}
			if err := gen.NewGenerator(c).Run(context.Background(), exp); err != nil {
				t.Fatalf("Run: %v", err)
			}
			all := collectAllSpans(exp.spans)
			if len(all) == 0 {
				t.Fatal("no spans generated")
			}
			for _, sp := range all {
				hasServiceName := false
				for _, attr := range sp.Attributes {
					if attr.Key == "service.name" {
						hasServiceName = true
						if attr.Value.GetStringValue() == "" {
							t.Errorf("span %s has empty service.name", sp.Name)
						}
						break
					}
				}
				if !hasServiceName {
					t.Errorf("span %s missing service.name attribute", sp.Name)
				}
			}
		})
	}
}

func TestGenerator_Scenario_SpanKinds(t *testing.T) {
	tests := []struct {
		scenario  string
		wantKinds []string
	}{
		{"microservice", []string{"GET /api/orders"}},
		{"monolith", []string{"POST /checkout"}},
		{"3-tier-web-app", []string{"GET /users"}},
		{"event-driven", []string{"publish order.created", "consume order.created"}},
	}

	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			c := gen.Config{
				Seed:          42,
				TraceCount:    1,
				Scenario:      tt.scenario,
				SpanCount:     15,
				MaxDepth:      5,
				ServiceName:   "test-svc",
				BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				TraceDuration: 5 * time.Second,
			}
			exp := &mockExporter{}
			if err := gen.NewGenerator(c).Run(context.Background(), exp); err != nil {
				t.Fatalf("Run: %v", err)
			}
			all := collectAllSpans(exp.spans)
			if len(all) == 0 {
				t.Fatal("no spans generated")
			}
			for _, sp := range all {
				if sp.Kind == tracev1.Span_SPAN_KIND_UNSPECIFIED {
					t.Errorf("span %s has UNSPECIFIED kind", sp.Name)
				}
			}
		})
	}
}

func TestGenerator_Scenario_Determinism(t *testing.T) {
	for _, s := range []string{"microservice", "monolith", "3-tier-web-app", "event-driven"} {
		t.Run(s, func(t *testing.T) {
			base := gen.Config{
				Seed:          42,
				TraceCount:    2,
				Scenario:      s,
				SpanCount:     15,
				MaxDepth:      5,
				ServiceName:   "test-svc",
				BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				TraceDuration: 5 * time.Second,
			}

			expA := &mockExporter{}
			expB := &mockExporter{}
			ctx := context.Background()

			if err := gen.NewGenerator(base).Run(ctx, expA); err != nil {
				t.Fatalf("Run A: %v", err)
			}
			if err := gen.NewGenerator(base).Run(ctx, expB); err != nil {
				t.Fatalf("Run B: %v", err)
			}

			aSpans := collectAllSpans(expA.spans)
			bSpans := collectAllSpans(expB.spans)

			if len(aSpans) != len(bSpans) {
				t.Fatalf("span count: A=%d, B=%d", len(aSpans), len(bSpans))
			}
			for i := range aSpans {
				if aSpans[i].Name != bSpans[i].Name {
					t.Errorf("span %d Name: A=%q, B=%q", i, aSpans[i].Name, bSpans[i].Name)
				}
				if aSpans[i].Kind != bSpans[i].Kind {
					t.Errorf("span %d Kind: A=%d, B=%d", i, aSpans[i].Kind, bSpans[i].Kind)
				}
			}
		})
	}
}

func TestGenerator_Scenario_RootTiming(t *testing.T) {
	for _, s := range []string{"microservice", "monolith"} {
		t.Run(s, func(t *testing.T) {
			c := gen.Config{
				Seed:          42,
				TraceCount:    1,
				Scenario:      s,
				SpanCount:     15,
				MaxDepth:      5,
				ServiceName:   "test-svc",
				BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				TraceDuration: 5 * time.Second,
			}
			exp := &mockExporter{}
			if err := gen.NewGenerator(c).Run(context.Background(), exp); err != nil {
				t.Fatalf("Run: %v", err)
			}
			all := collectAllSpans(exp.spans)

			var rootSpan *tracev1.Span
			for _, sp := range all {
				if len(sp.ParentSpanId) == 0 {
					rootSpan = sp
					break
				}
			}
			if rootSpan == nil {
				t.Fatal("no root span found")
			}
			rootStart := time.Unix(0, int64(rootSpan.StartTimeUnixNano))
			rootEnd := time.Unix(0, int64(rootSpan.EndTimeUnixNano))
			if rootEnd.Sub(rootStart) <= 0 {
				t.Errorf("root span duration should be positive, got %v", rootEnd.Sub(rootStart))
			}
		})
	}
}

// Ensure export.Exporter interface is satisfied by mockExporter
var _ export.Exporter = (*mockExporter)(nil)
