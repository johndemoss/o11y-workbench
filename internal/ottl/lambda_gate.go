package ottl

import (
	"fmt"

	"go.opentelemetry.io/collector/featuregate"
)

const ottlFunctionsEnableLambda = "ottl.functions.enableLambda"

// EnableLambdaExpressions turns on ottl.functions.enableLambda so OTTL statements
// may use lambda arguments with experimental converters.
func EnableLambdaExpressions() error {
	if err := featuregate.GlobalRegistry().Set(ottlFunctionsEnableLambda, true); err != nil {
		return fmt.Errorf("enable %s: %w", ottlFunctionsEnableLambda, err)
	}
	return nil
}
