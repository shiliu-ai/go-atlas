package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/shiliu-ai/go-atlas/aether/log"
)

// Locker is the minimal distributed lock the scheduler needs. It is satisfied
// structurally by *cache.RedisLock, so the scheduler does not import the cache
// pillar — the caller injects a lock via Job.Lock.
type Locker interface {
	// Acquire attempts to take the lock, returning true if acquired.
	Acquire(ctx context.Context) (bool, error)
	// Release releases the lock held by this instance.
	Release(ctx context.Context) error
}

// Job is a scheduled task.
type Job struct {
	// Name identifies the job in logs.
	Name string
	// Spec is a cron expression ("0 */5 * * *") or descriptor ("@every 30s").
	Spec string
	// Lock, if non-nil, makes the job single-executor across instances: on each
	// fire it runs only if the lock is acquired. Nil runs on every instance.
	//
	// The lock's TTL MUST exceed the job's worst-case run time. The scheduler
	// does not renew the lock mid-run, so a run that outlives the TTL lets the
	// lock expire and another instance may start the same job concurrently. Size
	// the TTL with headroom, and prefer idempotent jobs.
	Lock Locker
	// Run is the task body; a returned error is logged.
	Run func(ctx context.Context) error
}

type schedMetrics struct {
	once sync.Once
	runs metric.Int64Counter
}

func (m *Manager) recordRun(result string) {
	m.metrics.once.Do(func() {
		meter := otel.Meter("github.com/shiliu-ai/go-atlas/pillar/scheduler")
		m.metrics.runs, _ = meter.Int64Counter("scheduler.job.runs",
			metric.WithDescription("Scheduled job executions by result (success|skipped|failed)"))
	})
	if m.metrics.runs != nil {
		m.metrics.runs.Add(context.Background(), 1,
			metric.WithAttributes(attribute.String("result", result)))
	}
}

// Manager is the scheduler Pillar.
type Manager struct {
	cron   *cron.Cron
	logger log.Logger

	// ctx is the base context handed to each job run; cancel is called on Stop
	// so in-flight jobs can observe shutdown. Set in Init.
	ctx    context.Context
	cancel context.CancelFunc

	metrics schedMetrics
}

// Register schedules a job. Call it after atlas.New() and before Run().
// Returns an error if the job is incomplete or the spec is invalid.
func (m *Manager) Register(job Job) error {
	if job.Name == "" || job.Spec == "" || job.Run == nil {
		return fmt.Errorf("scheduler: job requires Name, Spec, and Run")
	}
	// Guard against overlapping runs of the same job on this instance: if a run
	// is still in flight when the next fire arrives (a job slower than its
	// interval), skip it rather than starting a second concurrent goroutine.
	var running atomic.Bool
	fn := func() {
		if !running.CompareAndSwap(false, true) {
			m.logger.Warn(m.baseCtx(), "scheduler job still running; skipping overlapping fire",
				log.F("job", job.Name))
			m.recordRun("skipped")
			return
		}
		defer running.Store(false)
		m.runJob(job)
	}
	if _, err := m.cron.AddFunc(job.Spec, fn); err != nil {
		return fmt.Errorf("scheduler: register %q: %w", job.Name, err)
	}
	return nil
}

// baseCtx returns the job base context, falling back to Background before Init.
func (m *Manager) baseCtx() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

// runJob executes one occurrence, applying the optional single-executor lock
// and recovering from panics so one bad run cannot crash the scheduler.
func (m *Manager) runJob(job Job) {
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if r := recover(); r != nil {
			m.logger.Error(ctx, "scheduler job panicked",
				log.F("job", job.Name), log.F("error", r))
			m.recordRun("failed")
		}
	}()

	if job.Lock != nil {
		acquired, err := job.Lock.Acquire(ctx)
		if err != nil {
			m.logger.Error(ctx, "scheduler job lock error",
				log.F("job", job.Name), log.F("error", err))
			m.recordRun("skipped")
			return
		}
		if !acquired {
			m.logger.Debug(ctx, "scheduler job skipped: lock held elsewhere",
				log.F("job", job.Name))
			m.recordRun("skipped")
			return
		}
		defer func() {
			if err := job.Lock.Release(ctx); err != nil {
				m.logger.Warn(ctx, "scheduler job lock release failed",
					log.F("job", job.Name), log.F("error", err))
			}
		}()
	}

	if err := job.Run(ctx); err != nil {
		m.logger.Error(ctx, "scheduler job failed",
			log.F("job", job.Name), log.F("error", err))
		m.recordRun("failed")
		return
	}
	m.recordRun("success")
}
