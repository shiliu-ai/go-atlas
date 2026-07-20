package atlas

// readinessState is the application's lifecycle readiness, served by /readyz.
//
// Following the liveness/readiness split used by Kubernetes probes and Spring
// Boot Actuator, /readyz reflects ONLY whether this instance should receive
// traffic right now — NOT the health of shared external dependencies (that is
// /healthz). Aggregating shared dependencies here would make every replica
// fail readiness together when a shared dependency blips, draining the whole
// service at once — the well-known readiness anti-pattern.
type readinessState int32

const (
	readinessStarting readinessState = iota // process up, not yet serving
	readinessReady                          // serving traffic
	readinessDraining                       // shutting down, refuse new traffic
)

func (s readinessState) String() string {
	switch s {
	case readinessReady:
		return "ready"
	case readinessDraining:
		return "draining"
	default:
		return "starting"
	}
}

// setReadiness atomically updates the readiness state.
func (a *Atlas) setReadiness(s readinessState) {
	a.readiness.Store(int32(s))
}

// readinessValue returns the current readiness state.
func (a *Atlas) readinessValue() readinessState {
	return readinessState(a.readiness.Load())
}
