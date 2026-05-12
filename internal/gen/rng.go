// Package gen provides RNG and ID generation for mock OTLP trace data.
// Topology decisions are deterministic (controlled by a config seed), while
// trace/span IDs use a random seed per run to ensure unique identifiers.
package gen

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	mrand "math/rand/v2"
	"time"
)

// SeededRNG wraps math/rand/v2 with a PCG source for deterministic generation.
// It is NOT safe for concurrent use — the caller must manage goroutine safety
// if multiple goroutines need to generate trace data.
type SeededRNG struct {
	r *mrand.Rand
}

// NewSeededRNG creates a new deterministic RNG from a seed value.
func NewSeededRNG(seed int64) *SeededRNG {
	return &SeededRNG{
		r: mrand.New(mrand.NewPCG(uint64(seed), uint64(seed))),
	}
}

// NewIDRng creates a new RNG seeded from crypto/rand so trace/span IDs
// are unique across runs. Falls back to nanosecond time if crypto/rand
// is unavailable (extremely rare on non-broken systems).
func NewIDRng() *SeededRNG {
	var seed int64
	if err := binary.Read(rand.Reader, binary.LittleEndian, &seed); err != nil {
		seed = time.Now().UnixNano()
	}
	return NewSeededRNG(seed)
}

// TraceID generates a deterministic 16-byte trace ID per OTLP/W3C spec.
// Retries if the result is all zeros (invalid per W3C Trace Context).
func (s *SeededRNG) TraceID() [16]byte {
	for {
		var id [16]byte
		for i := 0; i < 2; i++ {
			bits := s.r.Uint64()
			id[i*8] = byte(bits >> 56)
			id[i*8+1] = byte(bits >> 48)
			id[i*8+2] = byte(bits >> 40)
			id[i*8+3] = byte(bits >> 32)
			id[i*8+4] = byte(bits >> 24)
			id[i*8+5] = byte(bits >> 16)
			id[i*8+6] = byte(bits >> 8)
			id[i*8+7] = byte(bits)
		}
		if id != [16]byte{} {
			return id
		}
	}
}

// SpanID generates a deterministic 8-byte span ID per OTLP/W3C spec.
// Retries if the result is all zeros (invalid per W3C Trace Context).
func (s *SeededRNG) SpanID() [8]byte {
	for {
		bits := s.r.Uint64()
		id := [8]byte{
			byte(bits >> 56),
			byte(bits >> 48),
			byte(bits >> 40),
			byte(bits >> 32),
			byte(bits >> 24),
			byte(bits >> 16),
			byte(bits >> 8),
			byte(bits),
		}
		if id != [8]byte{} {
			return id
		}
	}
}

// Int63n returns a random int64 in [0, n). It panics if n <= 0.
func (s *SeededRNG) Int63n(n int64) int64 {
	return s.r.Int64N(n)
}

// Float64 returns a random float64 in [0.0, 1.0).
func (s *SeededRNG) Float64() float64 {
	return s.r.Float64()
}

// Duration returns a random duration in [0, max). It panics if max <= 0.
func (s *SeededRNG) Duration(max time.Duration) time.Duration {
	return time.Duration(s.Int63n(int64(max)))
}

// DurationRange returns a random duration in [min, max). It panics if max <= 0.
func (s *SeededRNG) DurationRange(min, max time.Duration) time.Duration {
	if min >= max {
		return min
	}
	return min + time.Duration(s.Int63n(int64(max-min)))
}

// Timestamp returns a random time in [base, base+maxOffset).
// It panics if maxOffset <= 0.
func (s *SeededRNG) Timestamp(base time.Time, maxOffset time.Duration) time.Time {
	offset := int64(maxOffset)
	return base.Add(time.Duration(s.r.Int64N(offset)))
}

// Bool returns a random boolean with roughly 50/50 distribution.
func (s *SeededRNG) Bool() bool {
	return s.r.IntN(2) == 1
}

// IntRange returns a random int in [min, max] inclusive.
// It panics if min > max.
func (s *SeededRNG) IntRange(min, max int) int {
	if min > max {
		panic(fmt.Sprintf("gen: IntRange: min (%d) > max (%d)", min, max))
	}
	n := max - min + 1
	return min + s.r.IntN(n)
}
