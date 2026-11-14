package engine

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
	xottlfuncs "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/xottl/ottlfuncs"
	"go.opentelemetry.io/collector/featuregate"
)

const ottlLambdaFeatureGateID = "ottl.functions.enableLambda"

// EnableOTTLLambdaGate turns on the upstream alpha ottl.functions.enableLambda gate.
// Call once during process startup before serving OTTL traffic.
func EnableOTTLLambdaGate() error {
	return featuregate.GlobalRegistry().Set(ottlLambdaFeatureGateID, true)
}

// ParserFunctions returns the standard OTTL function catalog plus experimental
// xottl converters (Filter, MapEach, Any, etc.) that accept lambda arguments.
func ParserFunctions[K any]() map[string]ottl.Factory[K] {
	return xottlfuncs.WithExperimentalConverters(ottlfuncs.StandardFuncs[K]())
}
