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
	runnableCalls   int
	runJobIDs       []uint64
	jobs            []*types.WikaURLRefreshJob
	runnableJobs    []*types.WikaURLRefreshJob
	runDueErr       error
	runnableErr     error
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

func (r *fakeURLRefreshRunner) RunRunnableJobs(ctx context.Context, now time.Time) ([]*types.WikaURLRefreshJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runnableCalls++
	if r.runnableErr != nil {
		return nil, r.runnableErr
	}
	return r.runnableJobs, nil
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

func TestWorkerRunsRunnablePendingJobsInSameTick(t *testing.T) {
	runner := &fakeURLRefreshRunner{
		jobs:         []*types.WikaURLRefreshJob{{ID: 101}},
		runnableJobs: []*types.WikaURLRefreshJob{{ID: 201}},
	}
	worker := NewWorker(runner, fakeFeatureGate{enabled: true}, WithWorkerID("worker-test"))

	if err := worker.RunOnce(context.Background(), time.Date(2026, 6, 30, 6, 15, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	if runner.runnableCalls != 1 {
		t.Fatalf("expected runnable jobs to be scanned once, got %d", runner.runnableCalls)
	}
	if len(runner.runJobIDs) != 2 || runner.runJobIDs[0] != 101 || runner.runJobIDs[1] != 201 {
		t.Fatalf("expected due and runnable jobs to run once each, got ids=%+v", runner.runJobIDs)
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
