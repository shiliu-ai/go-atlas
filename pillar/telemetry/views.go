package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// httpServerBuckets is the bucket set recommended by OTel HTTP semconv.
var httpServerBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// allowedHTTPAttrs is the semconv-approved attribute allow-list applied to
// HTTP server instruments. Anything else is dropped at the SDK layer.
var allowedHTTPAttrs = map[attribute.Key]struct{}{
	semconv.HTTPRequestMethodKey:      {},
	semconv.HTTPResponseStatusCodeKey: {},
	semconv.HTTPRouteKey:              {},
	semconv.URLSchemeKey:              {},
	semconv.NetworkProtocolVersionKey: {},
}

// httpServerDurationView overrides the default buckets on the HTTP server
// request duration histogram and strips attributes outside the allow-list.
func httpServerDurationView() sdkmetric.View {
	return sdkmetric.NewView(
		sdkmetric.Instrument{Name: "http.server.request.duration"},
		sdkmetric.Stream{
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: httpServerBuckets,
			},
			AttributeFilter: func(kv attribute.KeyValue) bool {
				_, ok := allowedHTTPAttrs[kv.Key]
				return ok
			},
		},
	)
}
