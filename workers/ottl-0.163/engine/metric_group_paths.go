package engine

// groupUsesQualifiedMetricPaths reports whether metric group execution needs
// qualified path context names. Groups execute inside their declared context,
// so implicit paths must resolve normally; enabling qualified names would
// require metric./datapoint./resource. prefixes and break Collector configs.
func groupUsesQualifiedMetricPaths(contextName string) bool {
	_ = contextName
	return false
}
