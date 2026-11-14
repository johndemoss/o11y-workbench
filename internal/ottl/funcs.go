package ottl

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
	xottlfuncs "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/xottl/ottlfuncs"
)

// parserFuncs returns the standard OTTL function catalog plus experimental
// lambda converters (Filter, MapEach, Any, etc.) from pkg/ottl/xottl.
func parserFuncs[K any]() map[string]ottl.Factory[K] {
	return xottlfuncs.WithExperimentalConverters(ottlfuncs.StandardFuncs[K]())
}
