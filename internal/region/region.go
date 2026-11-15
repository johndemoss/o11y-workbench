package region

import (
	"fmt"
	"sync"

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

// DefaultEndpoints returns the built-in collector destinations.
func DefaultEndpoints() []Region {
	return []Region{
		{Name: RegionLocal, Endpoint: "localhost:4317"},
		{Name: RegionUSEU, Endpoint: "us-eu-collector:4317"},
		{Name: RegionEUCent, Endpoint: "eu-cent-collector:4317"},
	}
}

var regionsMu sync.RWMutex
var allRegions []Name
var regions map[Name]*Region

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
	regionsMu.Lock()
	defer regionsMu.Unlock()
	if len(endpoints) == 0 {
		defaults := DefaultEndpoints()
		allRegions = make([]Name, len(defaults))
		regions = make(map[Name]*Region, len(defaults))
		for i := range defaults {
			r := defaults[i]
			allRegions[i] = r.Name
			regions[r.Name] = &r
		}
		return
	}

	allRegions = make([]Name, len(endpoints))
	regions = make(map[Name]*Region, len(endpoints))
	for i, ep := range endpoints {
		name := Name(fmt.Sprintf("endpoint-%d", i))
		allRegions[i] = name
		regions[name] = &Region{
			Name:     name,
			Endpoint: ep,
		}
	}
}

// All returns all available region/endpoint names.
func All() []Name {
	regionsMu.RLock()
	defer regionsMu.RUnlock()
	return append([]Name(nil), allRegions...)
}

// Assign deterministically assigns a trace to a region/endpoint based on seed and traceIndex.
func Assign(seed int64, traceIndex int) Name {
	regionsMu.RLock()
	defer regionsMu.RUnlock()
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
	regionsMu.RLock()
	defer regionsMu.RUnlock()
	if regions == nil {
		return ""
	}
	r, ok := regions[name]
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
	instanceID := fmt.Sprintf("o11y-workbench-%s", name)

	return []*commonv1.KeyValue{
		stringAttr("service.name", serviceName),
		stringAttr("service.instance.id", instanceID),
		stringAttr("cloud.region", string(name)),
		stringAttr("cloud.provider", "o11y-workbench"),
		stringAttr("telemetry.sdk.name", "o11y-workbench"),
		stringAttr("telemetry.sdk.language", "go"),
	}
}
