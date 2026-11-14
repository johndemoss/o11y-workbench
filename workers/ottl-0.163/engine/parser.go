// Package ottl provides OTTL (OpenTelemetry Transformation Language) statement
// validation and execution for the o11y-workbench web dashboard.
package engine

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
)

// ValidateResult holds the result of validating one or more OTTL statements.
type ValidateResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
	Parsed int      `json:"statements_parsed"`
}

// ValidateStatements parses and validates OTTL statements without executing them.
// It checks syntax, function existence, and argument types using the standard
// OTTL function catalog. Path expressions are accepted without validation
// (actual path validation requires a real telemetry context, which is added
// in Phase 2).
func ValidateStatements(statements []string) ValidateResult {
	if len(statements) == 0 {
		return ValidateResult{
			Valid:  false,
			Errors: []string{"no statements provided"},
		}
	}

	// Stub path parser — accepts any path expression by traversing the full
	// path chain via Next()/Keys(), satisfying ottl's isComplete() validation.
	// This lets us validate syntax and function arguments without needing a
	// full pdata TransformContext. Returns a no-op GetSetter.
	stubPathParser := func(path ottl.Path[any]) (ottl.GetSetter[any], error) {
		current := path
		for current != nil {
			_ = current.Keys() // mark keys as fetched (required by isComplete)
			current = current.Next()
		}
		return ottl.StandardGetSetter[any]{
			Getter: func(_ context.Context, _ any) (any, error) { return nil, nil },
			Setter: func(_ context.Context, _ any, _ any) error { return nil },
		}, nil
	}

	settings := component.TelemetrySettings{
		Logger: zap.NewNop(),
	}

	parser, err := ottl.NewParser[any](
		ParserFunctions[any](),
		stubPathParser,
		settings,
	)
	if err != nil {
		return ValidateResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("internal error creating parser: %v", err)},
		}
	}

	parsed, err := parser.ParseStatements(statements)
	if err != nil {
		return ValidateResult{
			Valid:  false,
			Errors: []string{err.Error()},
		}
	}

	return ValidateResult{
		Valid:  true,
		Parsed: len(parsed),
	}
}
