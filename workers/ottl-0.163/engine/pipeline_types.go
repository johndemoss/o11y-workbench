package engine

import (
	"fmt"
)

// ErrorMode defines how errors are handled during pipeline stage execution.
type ErrorMode string

const (
	ErrorModeIgnore    ErrorMode = "ignore"
	ErrorModeSilent    ErrorMode = "silent"
	ErrorModePropagate ErrorMode = "propagate"
)

// StageType defines the type of processor stage.
type StageType string

const (
	StageTypeTransform StageType = "transform"
	StageTypeFilter    StageType = "filter"
)

// ProcessorConfig holds the raw YAML configuration for a named processor.
type ProcessorConfig map[string]interface{}

// Stage represents a single pipeline stage referencing a processor.
type Stage struct {
	ID           string    `yaml:"id" json:"id"`
	Type         StageType `yaml:"type" json:"type"`
	ErrorMode    ErrorMode `yaml:"error_mode,omitempty" json:"error_mode,omitempty"`
	Signals      []string  `yaml:"signals,omitempty" json:"signals,omitempty"`
	ProcessorRef string    `yaml:"processor_ref" json:"processor_ref"`
}

// Pipeline is a complete pipeline definition with stages and processor configs.
type Pipeline struct {
	Stages          []Stage                    `yaml:"stages" json:"stages"`
	Processors      map[string]ProcessorConfig `yaml:"processors" json:"processors"`
	Receivers       []string                   `yaml:"receivers" json:"receivers"`
	Exporters       []string                   `yaml:"exporters" json:"exporters"`
	ReceiverConfigs map[string]interface{}     `yaml:"receiver_configs,omitempty" json:"receiver_configs,omitempty"`
	ExporterConfigs map[string]interface{}     `yaml:"exporter_configs,omitempty" json:"exporter_configs,omitempty"`
}

// PipelinesBySignal maps signal names (e.g. "traces", "logs", "metrics") to their Pipeline definitions.
type PipelinesBySignal map[string]*Pipeline

// PipelineConfig wraps a Pipeline with optional metadata.
type PipelineConfig struct {
	Pipeline Pipeline `yaml:"pipeline" json:"pipeline"`
}

// Validate checks the pipeline for common errors.
func (p *Pipeline) Validate() error {
	if len(p.Stages) == 0 {
		return fmt.Errorf("pipeline must have at least one stage")
	}

	seen := make(map[string]bool)
	for _, stage := range p.Stages {
		if stage.ID == "" {
			return fmt.Errorf("stage must have a non-empty id")
		}
		if seen[stage.ID] {
			return fmt.Errorf("duplicate stage id: %s", stage.ID)
		}
		seen[stage.ID] = true

		switch stage.Type {
		case StageTypeTransform, StageTypeFilter:
			// valid
		default:
			return fmt.Errorf("unknown stage type: %s", stage.Type)
		}

		if stage.ProcessorRef == "" {
			return fmt.Errorf("stage %s has no processor_ref", stage.ID)
		}
		if _, ok := p.Processors[stage.ProcessorRef]; !ok {
			return fmt.Errorf("stage %s references undefined processor: %s", stage.ID, stage.ProcessorRef)
		}
	}
	return nil
}
