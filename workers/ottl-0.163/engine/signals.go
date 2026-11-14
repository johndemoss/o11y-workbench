package engine

// Signals represents which telemetry signal types are in scope.
type Signals struct {
	Traces  bool `json:"traces"`
	Logs    bool `json:"logs"`
	Metrics bool `json:"metrics"`
}
