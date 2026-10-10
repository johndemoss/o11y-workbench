package gen

import "sync/atomic"

// Stats tracks attempted versus acknowledged export submissions.
// Available for workbench runs that enable a WorkloadProfile.
type Stats struct {
	AttemptedRequests uint64 `json:"attempted_requests"`
	AckedRequests     uint64 `json:"acked_requests"`
	FailedRequests    uint64 `json:"failed_requests"`
	AttemptedItems    uint64 `json:"attempted_items"`
	AckedItems        uint64 `json:"acked_items"`
	FailedItems       uint64 `json:"failed_items"`
}

type statsCounters struct {
	attemptedReq atomic.Uint64
	ackedReq     atomic.Uint64
	failedReq    atomic.Uint64
	attemptedIt  atomic.Uint64
	ackedIt      atomic.Uint64
	failedIt     atomic.Uint64
}

func (s *statsCounters) snapshot() Stats {
	return Stats{
		AttemptedRequests: s.attemptedReq.Load(),
		AckedRequests:     s.ackedReq.Load(),
		FailedRequests:    s.failedReq.Load(),
		AttemptedItems:    s.attemptedIt.Load(),
		AckedItems:        s.ackedIt.Load(),
		FailedItems:       s.failedIt.Load(),
	}
}

func (s *statsCounters) recordAttempt(items int) {
	if items < 0 {
		items = 0
	}
	s.attemptedReq.Add(1)
	s.attemptedIt.Add(uint64(items))
}

func (s *statsCounters) recordAck(items int) {
	if items < 0 {
		items = 0
	}
	s.ackedReq.Add(1)
	s.ackedIt.Add(uint64(items))
}

func (s *statsCounters) recordFail(items int) {
	if items < 0 {
		items = 0
	}
	s.failedReq.Add(1)
	s.failedIt.Add(uint64(items))
}
