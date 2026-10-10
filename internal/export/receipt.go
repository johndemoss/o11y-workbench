package export

import (
	"fmt"

	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// TransportAcceptanceOutcome classifies one export RPC response.
// These outcomes are transport/response acceptance only — not destination delivery.
type TransportAcceptanceOutcome string

const (
	// OutcomeFullSuccess means the RPC succeeded and no items were rejected.
	OutcomeFullSuccess TransportAcceptanceOutcome = "full_success"
	// OutcomePartialRejection means the RPC succeeded but some items were rejected.
	OutcomePartialRejection TransportAcceptanceOutcome = "partial_rejection"
	// OutcomeRPCFailure means the export RPC failed (no trustworthy acceptance split).
	OutcomeRPCFailure TransportAcceptanceOutcome = "rpc_failure"
	// OutcomeUncertain means the response cannot be interpreted into accepted/rejected counts.
	OutcomeUncertain TransportAcceptanceOutcome = "uncertain"
)

// ExportReceipt is typed transport acceptance accounting for one Export call.
// AcceptedItems is response acceptance, not observed destination delivery.
type ExportReceipt struct {
	Outcome        TransportAcceptanceOutcome `json:"outcome"`
	AttemptedItems int                        `json:"attempted_items"`
	AcceptedItems  int                        `json:"accepted_items"`
	RejectedItems  int                        `json:"rejected_items"`
	UncertainItems int                        `json:"uncertain_items,omitempty"`
	ErrorMessage   string                     `json:"error_message,omitempty"`
}

// ReceiptFullSuccess reports full transport acceptance of attempted items.
func ReceiptFullSuccess(attempted int) ExportReceipt {
	if attempted < 0 {
		attempted = 0
	}
	return ExportReceipt{
		Outcome:        OutcomeFullSuccess,
		AttemptedItems: attempted,
		AcceptedItems:  attempted,
	}
}

// ReceiptRPCFailure reports that the export RPC failed.
// The whole batch is failed at transport level; this is not a partial rejection.
func ReceiptRPCFailure(attempted int, err error) ExportReceipt {
	if attempted < 0 {
		attempted = 0
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return ExportReceipt{
		Outcome:        OutcomeRPCFailure,
		AttemptedItems: attempted,
		ErrorMessage:   msg,
	}
}

// ReceiptFromRejected maps an OTLP partial_success rejected count into a receipt.
// rejected == 0 is full success (warning messages may still be present).
// rejected > attempted is uncertain — never invent accepted/rejected totals.
func ReceiptFromRejected(attempted int, rejected int64, errMsg string) ExportReceipt {
	if attempted < 0 {
		attempted = 0
	}
	if rejected < 0 {
		return ExportReceipt{
			Outcome:        OutcomeUncertain,
			AttemptedItems: attempted,
			UncertainItems: attempted,
			ErrorMessage:   fmt.Sprintf("negative rejected count %d", rejected),
		}
	}
	if rejected == 0 {
		r := ReceiptFullSuccess(attempted)
		r.ErrorMessage = errMsg
		return r
	}
	if int64(attempted) < rejected {
		return ExportReceipt{
			Outcome:        OutcomeUncertain,
			AttemptedItems: attempted,
			UncertainItems: attempted,
			ErrorMessage:   fmt.Sprintf("rejected %d exceeds attempted %d: %s", rejected, attempted, errMsg),
		}
	}
	rej := int(rejected)
	return ExportReceipt{
		Outcome:        OutcomePartialRejection,
		AttemptedItems: attempted,
		AcceptedItems:  attempted - rej,
		RejectedItems:  rej,
		ErrorMessage:   errMsg,
	}
}

// CountSpans counts span items in an OTLP traces export batch.
func CountSpans(resources []*tracev1.ResourceSpans) int {
	n := 0
	for _, rs := range resources {
		if rs == nil {
			continue
		}
		for _, ss := range rs.ScopeSpans {
			if ss == nil {
				continue
			}
			n += len(ss.Spans)
		}
	}
	return n
}

// CountLogRecords counts log record items in an OTLP logs export batch.
func CountLogRecords(resources []*logsv1.ResourceLogs) int {
	n := 0
	for _, rl := range resources {
		if rl == nil {
			continue
		}
		for _, sl := range rl.ScopeLogs {
			if sl == nil {
				continue
			}
			n += len(sl.LogRecords)
		}
	}
	return n
}

// CountDataPoints counts metric data-point items in an OTLP metrics export batch.
func CountDataPoints(resources []*metricspb.ResourceMetrics) int {
	n := 0
	for _, rm := range resources {
		if rm == nil {
			continue
		}
		for _, sm := range rm.ScopeMetrics {
			if sm == nil {
				continue
			}
			for _, m := range sm.Metrics {
				if m == nil {
					continue
				}
				switch d := m.Data.(type) {
				case *metricspb.Metric_Gauge:
					if d.Gauge != nil {
						n += len(d.Gauge.DataPoints)
					}
				case *metricspb.Metric_Sum:
					if d.Sum != nil {
						n += len(d.Sum.DataPoints)
					}
				case *metricspb.Metric_Histogram:
					if d.Histogram != nil {
						n += len(d.Histogram.DataPoints)
					}
				case *metricspb.Metric_ExponentialHistogram:
					if d.ExponentialHistogram != nil {
						n += len(d.ExponentialHistogram.DataPoints)
					}
				case *metricspb.Metric_Summary:
					if d.Summary != nil {
						n += len(d.Summary.DataPoints)
					}
				}
			}
		}
	}
	return n
}
