package gen

import (
	"testing"
	"time"
)

func TestWorkloadPacerKeepsAbsoluteGrid(t *testing.T) {
	p := &workloadPacer{}
	origin := time.Unix(1_700_000_000, 0)
	interval := paceInterval(32)
	if send, _ := p.schedule(origin, interval); !send {
		t.Fatal("first start should export immediately")
	}
	now := origin.Add(200 * time.Microsecond)
	for i := 1; i <= 800; i++ {
		send, deadline := p.schedule(now, interval)
		if send {
			t.Fatalf("export %d: exported immediately at %s", i, now.Sub(origin))
		}
		want := origin.Add(time.Duration(i) * interval)
		if !deadline.Equal(want) {
			t.Fatalf("export %d deadline = %s, want %s", i, deadline.Sub(origin), want.Sub(origin))
		}
		now = want.Add(700 * time.Microsecond)
	}
}

func TestWorkloadPacerRebasesAfterMissedInterval(t *testing.T) {
	p := &workloadPacer{}
	origin := time.Unix(1_700_000_000, 0)
	interval := paceInterval(32)
	if send, _ := p.schedule(origin, interval); !send {
		t.Fatal("first start should export immediately")
	}
	jumped := origin.Add(4 * interval)
	send, deadline := p.schedule(jumped, interval)
	if !send || !deadline.IsZero() {
		t.Fatalf("missed interval send=%v deadline=%s", send, deadline.Sub(origin))
	}
	if !p.origin.Equal(jumped) {
		t.Fatalf("origin = %s, want %s", p.origin.Sub(origin), jumped.Sub(origin))
	}
}

func TestWorkloadPacerPreservesPreviousIntervalWhenBatchSizeChanges(t *testing.T) {
	p := &workloadPacer{}
	origin := time.Unix(1_700_000_000, 0)
	if send, _ := p.schedule(origin, paceInterval(32)); !send {
		t.Fatal("first start should export immediately")
	}
	tick := origin.Add(time.Millisecond)
	send, deadline := p.schedule(tick, paceInterval(16))
	if send || !deadline.Equal(origin.Add(paceInterval(32))) {
		t.Fatalf("send=%v deadline=%s", send, deadline.Sub(origin))
	}
}
