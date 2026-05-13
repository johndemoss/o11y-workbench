package region

import (
	"fmt"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
)

// Name represents a region (or endpoint) identifier.
type Name string

// Defined region names.
const (
	RegionLocal  Name = "local"
	RegionUSEU   Name = "us-eu"
	RegionEUCent Name = "eu-cent"
)

// defaultEndpoints are the built-in endpoints used when no custom ones are provided.
var defaultEndpoints = map[Name]string{
	RegionLocal:  "localhost:4317",
	RegionUSEU:   "us-eu-collector:4317",
	RegionEUCent: "eu-cent-collector:4317",
}

// allRegions is the ordered list of all configured regions/endpoints.
var allRegions []Name

// Regions maps region names to their full region definitions.
var Regions map[Name]*Region

// Region represents a configured region for trace routing.
type Region struct {
	Name     Name
	Endpoint string
}

func init() {
	SetEndpoints(nil)
}

// SetEndpoints replaces the default endpoints with a custom set.
// Each endpoint address becomes a destination; region names are auto-generated.
// If endpoints is nil or empty, the three built-in defaults are restored.
func SetEndpoints(endpoints []string) {
	if len(endpoints) == 0 {
		allRegions = []Name{RegionLocal, RegionUSEU, RegionEUCent}
		Regions = make(map[Name]*Region, len(allRegions))
		for _, name := range allRegions {
			Regions[name] = &Region{
				Name:     name,
				Endpoint: defaultEndpoints[name],
			}
		}
		return
	}

	allRegions = make([]Name, len(endpoints))
	Regions = make(map[Name]*Region, len(endpoints))
	for i, ep := range endpoints {
		name := Name(fmt.Sprintf("endpoint-%d", i))
		allRegions[i] = name
		Regions[name] = &Region{
			Name:     name,
			Endpoint: ep,
		}
	}
}

// All returns all available region/endpoint names.
func All() []Name {
	return allRegions
}

// Assign deterministically assigns a trace to a region/endpoint based on seed and traceIndex.
func Assign(seed int64, traceIndex int) Name {
	n := len(allRegions)
	if n == 0 {
		return ""
	}
	idx := int((seed ^ int64(traceIndex)) % int64(n))
	if idx < 0 {
		idx = -idx
	}
	return allRegions[idx]
}

// GetEndpoint returns the collector endpoint URL for the given region name.
func GetEndpoint(name Name) string {
	if Regions == nil {
		return ""
	}
	r, ok := Regions[name]
	if !ok {
		return ""
	}
	return r.Endpoint
}

// stringAttr creates a KeyValue with a string AnyValue.
func stringAttr(key, value string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key: key,
		Value: &commonv1.AnyValue{
			Value: &commonv1.AnyValue_StringValue{
				StringValue: value,
			},
		},
	}
}

// ResourceAttributes returns resource attributes for a region.
func ResourceAttributes(name Name, serviceName string) []*commonv1.KeyValue {
	instanceID := fmt.Sprintf("otel-workbench-%s", name)

	return []*commonv1.KeyValue{
		stringAttr("service.name", serviceName),
		stringAttr("service.instance.id", instanceID),
		stringAttr("cloud.region", string(name)),
		stringAttr("cloud.provider", "otel-workbench"),
		stringAttr("telemetry.sdk.name", "otel-workbench"),
		stringAttr("telemetry.sdk.language", "go"),
	}
}
