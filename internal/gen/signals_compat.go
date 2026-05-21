package gen

// DeriveSignalType converts Signals back to legacy string format.
// Used for backwards compatibility in API responses during the migration period.
// This allows old clients that expect signal_type strings to continue working.
func DeriveSignalType(s Signals) string {
	t, l, m := s.Traces, s.Logs, s.Metrics

	if t && l && m {
		return "all"
	}
	if t && l {
		return "both"
	}
	if t && m {
		return "traces+metrics"
	}
	if l && m {
		return "logs+metrics"
	}
	if l {
		return "logs"
	}
	if m {
		return "metrics"
	}
	return "traces"
}
