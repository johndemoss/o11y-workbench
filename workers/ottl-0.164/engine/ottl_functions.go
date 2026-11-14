package engine

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
	xottlfuncs "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/xottl/ottlfuncs"
)

// parserFunctions returns the standard OTTL function map plus experimental xottl converters.
func parserFunctions[K any]() map[string]ottl.Factory[K] {
	return xottlfuncs.WithExperimentalConverters(ottlfuncs.StandardFuncs[K]())
}
