package urlrefresh

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeURLRefreshRunner struct {
	mu              sync.Mutex
	dueCalls        int
	runCalls        int
	runJobIDs       []uint64
	jobs            []*types.WikaURLRefreshJob
	runDueErr       error
	runJobErr       error
	lastRunJobInput RunJobInput
}

func (r *fakeURLRefreshRunner) RunDueSchedules(ctx context.Context, now time.Time) ([]*types.WikaURLRefreshJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dueCalls++
	if r.runDueErr != nil {
		return nil, r.runDueErr
	}
	return r.jobs, nil
}

func (r *fakeURLRefreshRunner) RunJob(ctx context.Context, input RunJobInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runCalls++
	r.runJobIDs = append(r.runJobIDs, input.JobID)
	r.lastRunJobInput = input
	return r.runJobErr
}

func (r *fakeURLRefreshRunner) Counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dueCalls, r.runCalls
}

func waitForURLRefreshWorkerCalls(t *testing.T, runner *fakeURLRefreshRunner, wantRunJobs int) {
	t.Helper()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, runCalls := runner.Counts()
		if runCalls >= wantRunJobs {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	_, runCalls := runner.Counts()
	t.Fatalf("expected at least %d run-job calls, got %d", wantRunJobs, runCalls)
}

func TestWorkerCreatesDueJobsAndRunsThemUntilStopped(t *testing.T) {
	runner := &fakeURLRefreshRunner{jobs: []*types.WikaURLRefreshJob{{ID: 101}, {ID: 102}}}
	worker := NewWorker(runner, fakeFeatureGate{enabled: true}, WithWorkerInterval(10*time.Millisecond), WithWorkerID("worker-test"))

	worker.Start(context.Background())
	waitForURLRefreshWorkerCalls(t, runner, 2)

	worker.Stop()
	_, stoppedRunCalls := runner.Counts()
	time.Sleep(30 * time.Millisecond)
	_, gotRunCalls := runner.Counts()
	if gotRunCalls != stoppedRunCalls {
		t.Fatalf("expected worker to stop at %d run-job calls, got %d", stoppedRunCalls, gotRunCalls)
	}
	if runner.runJobIDs[0] != 101 || runner.runJobIDs[1] != 102 || runner.lastRunJobInput.WorkerID != "worker-test" {
		t.Fatalf("unexpected run-job inputs: ids=%+v last=%+v", runner.runJobIDs, runner.lastRunJobInput)
	}
}

func TestWorkerDoesNotRunWhenFeatureDisabled(t *testing.T) {
	runner := &fakeURLRefreshRunner{jobs: []*types.WikaURLRefreshJob{{ID: 101}}}
	worker := NewWorker(runner, fakeFeatureGate{enabled: false}, WithWorkerInterval(10*time.Millisecond))

	worker.Start(context.Background())
	defer worker.Stop()
	time.Sleep(30 * time.Millisecond)

	dueCalls, runCalls := runner.Counts()
	if dueCalls != 0 || runCalls != 0 {
		t.Fatalf("feature disabled worker should not run due schedules or jobs, due=%d run=%d", dueCalls, runCalls)
	}
}
