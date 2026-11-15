package gen

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ValidateConfig checks dimensions before a generator allocates or starts work.
// Workload batches are limited to 100,000 items and an estimated 64 MiB,
// including 512 bytes per item and 128 bytes per attribute plus its value.
func ValidateConfig(c Config) error {
	if len(c.Regions) == 0 {
		return fmt.Errorf("no regions configured")
	}
	if c.Signals == (Signals{}) {
		return fmt.Errorf("no signals selected")
	}
	p := c.Workload
	if c.TraceCount < 0 || c.Rate < 0 || c.SpanCount < 0 || c.MaxDepth < 0 || c.ScenarioSpanCount < 0 || c.Duration < 0 || c.TraceDuration < 0 || c.ErrorPercent < 0 || c.ErrorPercent > 100 {
		return fmt.Errorf("invalid negative generation dimension or error percentage")
	}
	if c.LogConfig.LogCount < 0 || c.LogConfig.LogsPerSpan < 0 || c.MetricConfig.MetricCount < 0 {
		return fmt.Errorf("invalid negative signal count")
	}
	if err := c.LogConfig.validate(); err != nil {
		return err
	}
	if c.TraceCount == 0 && c.Rate > 0 && c.Duration > 0 && float64(c.Rate)*c.Duration.Seconds() >= float64(math.MaxInt) {
		return fmt.Errorf("rate and duration exceed trace count limit")
	}
	if c.SpanCount > 100000 || c.ScenarioSpanCount > 100000 || c.MaxDepth > 1000 {
		return fmt.Errorf("generation span count or depth exceeds limit")
	}
	if p.ItemsPerRequest < 0 || p.ItemsPerRequest > 100000 || p.AttributeCount < 0 || p.AttributeCount > 1024 || p.AttributeValueBytes < 0 || p.AttributeValueBytes > 1048576 || p.Cardinality < 0 {
		return fmt.Errorf("invalid workload dimensions")
	}
	if c.TraceCount > 0 && uint64(c.TraceCount) > math.MaxUint64/uint64(p.itemsPerRequest()) {
		return fmt.Errorf("workload item sequence would overflow")
	}
	items := p.itemsPerRequest()
	if !p.flatShape() && !IsWorkloadScenario(c.Scenario) && p.ItemsPerRequest == 0 {
		items = max(items, c.SpanCount, c.ScenarioSpanCount)
	}
	estimated := int64(items) * (512 + int64(p.AttributeCount)*(128+int64(p.AttributeValueBytes)))
	if estimated > 64<<20 {
		return fmt.Errorf("estimated workload batch allocation exceeds 64 MiB")
	}
	for _, rate := range []float64{p.RateItemsPerSec, p.RequestsPerSec, p.Burst.PeakRateItemsPerSec} {
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || (rate > 0 && rate < float64(time.Second)*float64(items)/float64(math.MaxInt64)) {
			return fmt.Errorf("invalid workload rate")
		}
	}
	if p.Burst.On < 0 || p.Burst.Off < 0 || p.Burst.On > time.Duration(math.MaxInt64)-p.Burst.Off {
		return fmt.Errorf("invalid burst duration")
	}
	if strings.EqualFold(p.Mode, "burst") && (p.Burst.Off <= 0 || p.Burst.PeakRateItemsPerSec == 0) {
		return fmt.Errorf("burst mode requires positive off duration and peak rate")
	}
	return nil
}
