package urlrefresh

import (
	"context"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type dueJobRunner interface {
	RunDueSchedules(ctx context.Context, now time.Time) ([]*types.WikaURLRefreshJob, error)
	RunJob(ctx context.Context, input RunJobInput) error
}

type Worker struct {
	runner        dueJobRunner
	flags         FeatureGate
	interval      time.Duration
	workerID      string
	leaseDuration time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

type WorkerOption func(*Worker)

func WithWorkerInterval(interval time.Duration) WorkerOption {
	return func(w *Worker) {
		if interval > 0 {
			w.interval = interval
		}
	}
}

func WithWorkerID(workerID string) WorkerOption {
	return func(w *Worker) {
		if workerID != "" {
			w.workerID = workerID
		}
	}
}

func WithWorkerLeaseDuration(duration time.Duration) WorkerOption {
	return func(w *Worker) {
		if duration > 0 {
			w.leaseDuration = duration
		}
	}
}

func NewWorker(runner dueJobRunner, flags FeatureGate, opts ...WorkerOption) *Worker {
	worker := &Worker{
		runner:        runner,
		flags:         flags,
		interval:      time.Minute,
		workerID:      "wika-url-refresh-worker",
		leaseDuration: time.Minute,
	}
	for _, opt := range opts {
		opt(worker)
	}
	return worker
}

func (w *Worker) Start(ctx context.Context) {
	if !w.featureEnabled(ctx) {
		return
	}
	w.mu.Lock()
	if w.cancel != nil {
		w.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	w.cancel = cancel
	w.done = done
	w.mu.Unlock()

	go func() {
		defer close(done)
		w.RunOnce(runCtx, time.Now())
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case now := <-ticker.C:
				w.RunOnce(runCtx, now)
			}
		}
	}()
}

func (w *Worker) Stop() {
	w.mu.Lock()
	cancel := w.cancel
	done := w.done
	w.cancel = nil
	w.done = nil
	w.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (w *Worker) RunOnce(ctx context.Context, now time.Time) error {
	if w.runner == nil || !w.featureEnabled(ctx) {
		return nil
	}
	jobs, err := w.runner.RunDueSchedules(ctx, now)
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now()
	}
	for _, job := range jobs {
		if job == nil {
			continue
		}
		if err := w.runner.RunJob(ctx, RunJobInput{
			JobID:         job.ID,
			WorkerID:      w.workerID,
			Now:           now,
			LeaseDuration: w.leaseDuration,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) featureEnabled(ctx context.Context) bool {
	if w.flags == nil {
		return false
	}
	return w.flags.GetBool(ctx, urlRefreshFeatureFlagKey, "", false)
}
