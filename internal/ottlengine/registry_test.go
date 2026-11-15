package ottlengine

import (
	"context"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

func TestRegistryDefaultAndGet(t *testing.T) {
	local := &LocalEngine{}
	r := NewRegistry(DefaultContribVersion, local)

	if got := r.Default(); got != DefaultContribVersion {
		t.Fatalf("Default() = %q, want %q", got, DefaultContribVersion)
	}

	eng, err := r.Get("")
	if err != nil {
		t.Fatalf("Get(\"\"): %v", err)
	}
	if eng.Info().Contrib != DefaultContribVersion {
		t.Fatalf("empty version engine contrib = %q", eng.Info().Contrib)
	}
	if eng.Info().Source != "in-process" {
		t.Fatalf("source = %q, want in-process", eng.Info().Source)
	}

	eng, err = r.Get(DefaultContribVersion)
	if err != nil {
		t.Fatalf("Get(%s): %v", DefaultContribVersion, err)
	}
	if eng != local {
		t.Fatal("Get(default) did not return local engine")
	}
}

func TestRegistryUnknownVersion(t *testing.T) {
	r := NewRegistry(DefaultContribVersion, &LocalEngine{})
	_, err := r.Get("9.9.9")
	if err == nil {
		t.Fatal("expected error for unknown version")
	}
	want := "unsupported collector_version: 9.9.9"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestRegistryListIncludesLocal(t *testing.T) {
	r := NewRegistry(DefaultContribVersion, &LocalEngine{})
	list := r.List()
	if len(list) == 0 {
		t.Fatal("List() empty")
	}
	found := false
	for _, v := range list {
		if v.Contrib == DefaultContribVersion && v.Source == "in-process" {
			found = true
			if v.Stable != DefaultStableVersion {
				t.Fatalf("stable = %q, want %q", v.Stable, DefaultStableVersion)
			}
		}
	}
	if !found {
		t.Fatalf("List() missing local engine: %+v", list)
	}
}

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry(DefaultContribVersion, &LocalEngine{})
	wc := NewWorkerClient("0.164.0", "1.70.0", "http://127.0.0.1:9")
	r.Register("0.164.0", wc)
	eng, err := r.Get("0.164.0")
	if err != nil {
		t.Fatalf("Get(0.164.0): %v", err)
	}
	if eng.Info().Source != "worker" {
		t.Fatalf("source = %q", eng.Info().Source)
	}
	list := r.List()
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
}

func TestParseWorkerBindings(t *testing.T) {
	b, err := parseWorkerBindings("0.164.0=/tmp/w,0.163.0=/bin/x")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 2 || b[0].Version != "0.164.0" || b[0].Path != "/tmp/w" {
		t.Fatalf("unexpected bindings: %+v", b)
	}
	if _, err := parseWorkerBindings("bad"); err == nil {
		t.Fatal("expected error for invalid binding")
	}
}

func TestContribToStable(t *testing.T) {
	if got := contribToStable("0.165.0"); got != "1.71.0" {
		t.Fatalf("contribToStable(0.165.0) = %q, want 1.71.0", got)
	}
	if got := contribToStable("0.164.0"); got != "1.70.0" {
		t.Fatalf("contribToStable(0.164.0) = %q, want 1.70.0", got)
	}
	if got := contribToStable("0.163.0"); got != "1.69.0" {
		t.Fatalf("contribToStable(0.163.0) = %q, want 1.69.0", got)
	}
}

func TestLocalValidatePipelineOTTLEmpty(t *testing.T) {
	e := &LocalEngine{}
	res := e.ValidatePipelineOTTL(context.Background(), ottl.Pipeline{Processors: map[string]ottl.ProcessorConfig{}})
	if !res.Valid || res.Parsed != 0 {
		t.Fatalf("empty processors: %+v", res)
	}
}

func TestLocalValidatePipelineOTTLStatements(t *testing.T) {
	e := &LocalEngine{}
	good := ottl.Pipeline{
		Processors: map[string]ottl.ProcessorConfig{
			"p1": {"statements": []interface{}{`set(name, "x")`}},
		},
	}
	res := e.ValidatePipelineOTTL(context.Background(), good)
	if !res.Valid {
		t.Fatalf("valid statement failed: %+v", res)
	}
	if res.Parsed < 1 {
		t.Fatalf("parsed = %d, want >= 1", res.Parsed)
	}

	bad := ottl.Pipeline{
		Processors: map[string]ottl.ProcessorConfig{
			"p1": {"statements": []interface{}{`this is not valid ottl`}},
		},
	}
	res = e.ValidatePipelineOTTL(context.Background(), bad)
	if res.Valid {
		t.Fatal("expected invalid statement to fail")
	}
	if len(res.Errors) == 0 {
		t.Fatal("expected errors")
	}
}

func TestRegistryWithdrawsExitedWorker(t *testing.T) {
	done := make(chan struct{})
	client := NewWorkerClient("0.163.0", "1.69.0", "http://127.0.0.1:1")
	client.done = done
	r := NewRegistry(DefaultContribVersion, &LocalEngine{})
	r.Register("0.163.0", client)
	close(done)
	if _, err := r.Get("0.163.0"); err == nil {
		t.Fatal("dead worker still resolves")
	}
	if len(r.List()) != 1 {
		t.Fatalf("dead worker still advertised: %v", r.List())
	}
}
