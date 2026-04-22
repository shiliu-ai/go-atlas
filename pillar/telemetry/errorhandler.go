package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"

	"github.com/shiliu-ai/go-atlas/aether/log"
)

// newOTelErrorHandler routes OTel SDK internal errors (OTLP export failures,
// batch processor drops, reader collection errors) into the Atlas logger.
//
// Without this hook OTel writes to stderr by default, which means a broken
// collector connection or a silently-dropping batch queue never surfaces in
// the application's log stream — the exact scenario where business alerts
// built on top of these metrics would quietly stop firing. Plumbing through
// log.Logger lets teams attach log-based alerts as a second line of defence.
func newOTelErrorHandler(logger log.Logger) otel.ErrorHandler {
	return otel.ErrorHandlerFunc(func(err error) {
		if err == nil {
			return
		}
		logger.Error(context.Background(), "telemetry: otel internal error", log.F("error", err.Error()))
	})
}
