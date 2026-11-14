package engine

import (
	"go.opentelemetry.io/collector/featuregate"
)

const ottlEnableLambdaGateID = "ottl.functions.enableLambda"

// EnableOTTLLambdaFunctions enables the upstream ottl.functions.enableLambda gate.
// Call once during process startup before OTTL validation or execution.
func EnableOTTLLambdaFunctions() error {
	return featuregate.GlobalRegistry().Set(ottlEnableLambdaGateID, true)
}
