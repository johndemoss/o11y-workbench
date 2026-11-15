package gen

import (
	"context"
	"math"
	"strings"
	"time"
)

// workloadPacer spaces export starts. Export time counts toward the interval.
// Deadlines advance from the previous scheduled start to limit drift.
type workloadPacer struct {
	profile      WorkloadProfile
	fallbackRate int
	origin       time.Time
	interval     time.Duration
}

func (p *workloadPacer) start(ctx context.Context, items int) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		now := time.Now()
		rate := p.profile.requestRate(now)
		if strings.EqualFold(p.profile.Mode, "burst") && rate <= 0 {
			p.origin = time.Time{}
			p.interval = 0
			timer := time.NewTimer(10 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if strings.EqualFold(p.profile.Mode, "burst") {
			rate = p.profile.Burst.PeakRateItemsPerSec / float64(items)
		} else if p.profile.RequestsPerSec == 0 && p.profile.RateItemsPerSec > 0 {
			rate = p.profile.RateItemsPerSec / float64(items)
		} else if rate <= 0 && p.fallbackRate > 0 {
			rate = float64(p.fallbackRate)
		}
		sendNow, deadline := p.schedule(now, paceInterval(rate))
		if sendNow {
			return nil
		}
		if err := waitUntil(ctx, deadline); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if strings.EqualFold(p.profile.Mode, "burst") && p.profile.requestRate(time.Now()) <= 0 {
			continue
		}
		return nil
	}
}

func paceInterval(rate float64) time.Duration {
	if rate <= 0 {
		return 0
	}
	nanos := float64(time.Second) / rate
	if nanos >= float64(math.MaxInt64) {
		return time.Duration(math.MaxInt64)
	}
	interval := time.Duration(nanos)
	if interval <= 0 {
		return time.Nanosecond
	}
	return interval
}

// schedule returns the next deadline, resetting after a missed interval.
func (p *workloadPacer) schedule(now time.Time, interval time.Duration) (sendNow bool, deadline time.Time) {
	if interval <= 0 || p.origin.IsZero() {
		p.origin = now
		p.interval = interval
		return true, time.Time{}
	}
	deadline = p.origin.Add(p.interval)
	if !now.Before(deadline) {
		p.origin = now
		p.interval = interval
		return true, time.Time{}
	}
	p.origin = deadline
	p.interval = interval
	return false, deadline
}

func waitUntil(ctx context.Context, deadline time.Time) error {
	delay := time.Until(deadline)
	if delay <= 0 {
		return paceCtx(ctx)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return paceCtx(ctx)
	case <-timer.C:
		return nil
	}
}

func paceCtx(ctx context.Context) error {
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return nil
		}
		return ctx.Err()
	default:
		return nil
	}
}
