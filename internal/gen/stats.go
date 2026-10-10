package gen

import (
	"sync/atomic"

	"github.com/johndemoss/o11y-workbench/internal/export"
)

// Stats tracks attempted versus transport-accepted export submissions.
// Available for workbench runs that enable a WorkloadProfile.
//
// AckedItems / Accepted semantics are OTLP/export response acceptance only.
// They are not destination delivery (see T18 unique receipts).
type Stats struct {
	AttemptedRequests uint64 `json:"attempted_requests"`
	AckedRequests     uint64 `json:"acked_requests"`
	FailedRequests    uint64 `json:"failed_requests"`
	AttemptedItems    uint64 `json:"attempted_items"`
	// AckedItems is transport-accepted item count (response acceptance).
	AckedItems uint64 `json:"acked_items"`
	// RejectedItems is partial_success rejection count (not full-batch failure).
	RejectedItems uint64 `json:"rejected_items"`
	FailedItems   uint64 `json:"failed_items"`
	// UncertainItems is acceptance that could not be split into accepted/rejected.
	UncertainItems uint64 `json:"uncertain_items"`
}

type statsCounters struct {
	attemptedReq atomic.Uint64
	ackedReq     atomic.Uint64
	failedReq    atomic.Uint64
	attemptedIt  atomic.Uint64
	ackedIt      atomic.Uint64
	rejectedIt   atomic.Uint64
	failedIt     atomic.Uint64
	uncertainIt  atomic.Uint64
}

func (s *statsCounters) snapshot() Stats {
	return Stats{
		AttemptedRequests: s.attemptedReq.Load(),
		AckedRequests:     s.ackedReq.Load(),
		FailedRequests:    s.failedReq.Load(),
		AttemptedItems:    s.attemptedIt.Load(),
		AckedItems:        s.ackedIt.Load(),
		RejectedItems:     s.rejectedIt.Load(),
		FailedItems:       s.failedIt.Load(),
		UncertainItems:    s.uncertainIt.Load(),
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

// recordReceipt accounts one typed transport-acceptance receipt.
// Partial rejection does not become full-batch failure or full-batch acceptance.
func (s *statsCounters) recordReceipt(r export.ExportReceipt) {
	switch r.Outcome {
	case export.OutcomeFullSuccess:
		s.ackedReq.Add(1)
		if r.AcceptedItems > 0 {
			s.ackedIt.Add(uint64(r.AcceptedItems))
		}
	case export.OutcomePartialRejection:
		s.ackedReq.Add(1)
		if r.AcceptedItems > 0 {
			s.ackedIt.Add(uint64(r.AcceptedItems))
		}
		if r.RejectedItems > 0 {
			s.rejectedIt.Add(uint64(r.RejectedItems))
		}
		if r.UncertainItems > 0 {
			s.uncertainIt.Add(uint64(r.UncertainItems))
		}
	case export.OutcomeRPCFailure:
		s.failedReq.Add(1)
		if r.AttemptedItems > 0 {
			s.failedIt.Add(uint64(r.AttemptedItems))
		}
	case export.OutcomeUncertain:
		// RPC completed but acceptance split is unusable.
		s.ackedReq.Add(1)
		uncertain := r.UncertainItems
		if uncertain <= 0 {
			uncertain = r.AttemptedItems
		}
		if uncertain > 0 {
			s.uncertainIt.Add(uint64(uncertain))
		}
	default:
		s.ackedReq.Add(1)
		if r.AttemptedItems > 0 {
			s.uncertainIt.Add(uint64(r.AttemptedItems))
		}
	}
}
