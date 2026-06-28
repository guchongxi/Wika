package evalschedule

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeEvalScheduleRunner struct {
	mu    sync.Mutex
	calls int
}

func (r *fakeEvalScheduleRunner) RunDueSchedules(ctx context.Context, now time.Time) ([]*types.WikaEvalRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return nil, nil
}

func (r *fakeEvalScheduleRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func waitForWorkerCalls(t *testing.T, runner *fakeEvalScheduleRunner, want int) {
	t.Helper()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if runner.CallCount() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected at least %d worker calls, got %d", want, runner.CallCount())
}

func TestWorkerStartRunsDueSchedulesAndStopPreventsFurtherRuns(t *testing.T) {
	runner := &fakeEvalScheduleRunner{}
	worker := NewWorker(runner, fakeEvalScheduleGate{enabled: true}, WithWorkerInterval(10*time.Millisecond))

	worker.Start(context.Background())
	waitForWorkerCalls(t, runner, 1)

	worker.Stop()
	stoppedAt := runner.CallCount()
	time.Sleep(30 * time.Millisecond)
	if got := runner.CallCount(); got != stoppedAt {
		t.Fatalf("expected worker to stop at %d calls, got %d", stoppedAt, got)
	}
}

func TestWorkerDoesNotCallRunnerWhenFeatureDisabled(t *testing.T) {
	runner := &fakeEvalScheduleRunner{}
	worker := NewWorker(runner, fakeEvalScheduleGate{enabled: false}, WithWorkerInterval(10*time.Millisecond))

	worker.Start(context.Background())
	defer worker.Stop()
	time.Sleep(30 * time.Millisecond)
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("feature disabled worker should not call runner, got %d calls", got)
	}
}
