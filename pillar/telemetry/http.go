package telemetry

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// unknownRoute is the placeholder used when gin cannot resolve the route
// template (404s and unmatched paths). Keeping it bounded prevents the
// http.route label from exploding on malicious probing traffic.
const unknownRoute = "UNKNOWN"

// httpMiddleware constructs the otelgin middleware with our defaults:
//   - route label set from c.FullPath() with an UNKNOWN fallback
//   - TracerProvider/MeterProvider pinned to the Pillar's instances so tests
//     that use a manual reader see the recorded metrics
func (t *Telemetry) httpMiddleware() gin.HandlerFunc {
	opts := []otelgin.Option{
		otelgin.WithTracerProvider(t.tp),
		otelgin.WithMeterProvider(t.mp),
		otelgin.WithGinMetricAttributeFn(func(c *gin.Context) []attribute.KeyValue {
			route := c.FullPath()
			if route == "" {
				route = unknownRoute
			}
			return []attribute.KeyValue{semconv.HTTPRoute(route)}
		}),
	}
	return otelgin.Middleware(t.serviceName, opts...)
}
