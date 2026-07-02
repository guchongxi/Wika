package urlrefresh

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/wika/governance/urlrefresh/safefetch"
	"github.com/Tencent/WeKnora/internal/wika/governance/version"
	"github.com/stretchr/testify/require"
)

type fakeFetcher struct {
	result *safefetch.FetchResult
	err    error
}

func (f fakeFetcher) Fetch(ctx context.Context, raw string) (*safefetch.FetchResult, error) {
	return f.result, f.err
}

type fakeValidatingFetcher struct {
	fakeFetcher
	validateErr   error
	validateCalls []string
}

func (f *fakeValidatingFetcher) ValidateURL(ctx context.Context, raw string) error {
	f.validateCalls = append(f.validateCalls, raw)
	return f.validateErr
}

type fakeKnowledgeUpdater struct {
	knowledgeID string
	payload     *types.ManualKnowledgePayload
	updated     *types.Knowledge
}

func (u *fakeKnowledgeUpdater) UpdateManualKnowledge(ctx context.Context, knowledgeID string, payload *types.ManualKnowledgePayload) (*types.Knowledge, error) {
	u.knowledgeID = knowledgeID
	u.payload = payload
	if u.updated != nil {
		return u.updated, nil
	}
	return &types.Knowledge{ID: knowledgeID, TenantID: 90, KnowledgeBaseID: "kb-url", Title: payload.Title, Type: types.KnowledgeTypeManual, EnableStatus: payload.Status}, nil
}

type fakeVersionRecorder struct {
	input *version.RecordVersionInput
}

func (r *fakeVersionRecorder) RecordVersion(ctx context.Context, input version.RecordVersionInput) (*types.WikaKnowledgeVersion, error) {
	r.input = &input
	return &types.WikaKnowledgeVersion{ID: 700, KnowledgeID: input.KnowledgeID, TenantID: input.TenantID, KBID: input.KBID, VersionNo: 2}, nil
}

type fakeURLRefreshAudit struct {
	entries []*types.AuditLog
}

func (a *fakeURLRefreshAudit) Log(ctx context.Context, entry *types.AuditLog) error {
	a.entries = append(a.entries, entry)
	return nil
}

type fakeFeatureGate struct {
	enabled bool
}

func (g fakeFeatureGate) GetBool(ctx context.Context, key string, envName string, def bool) bool {
	return g.enabled
}

func TestServiceRunJobFetchesIntoPendingReviewWithoutUpdatingKnowledge(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	job, err := store.CreateJob(context.Background(), CreateJobInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		Now:         now,
	})
	require.NoError(t, err)

	svc := NewService(store, fakeFetcher{result: &safefetch.FetchResult{
		Text:        "新抓取内容",
		ContentType: "text/plain",
		FinalURL:    "https://example.com/doc",
		SizeBytes:   int64(len("新抓取内容")),
	}}, WithFeatureGate(fakeFeatureGate{enabled: true}))
	require.NoError(t, svc.RunJob(context.Background(), RunJobInput{
		JobID:         job.ID,
		WorkerID:      "worker-1",
		Now:           now,
		LeaseDuration: time.Minute,
	}))

	var updated types.WikaURLRefreshJob
	require.NoError(t, db.First(&updated, "id = ?", job.ID).Error)
	if updated.Status != JobStatusPendingReview || updated.FetchedContent != "新抓取内容" || updated.FetchedHash == "" {
		t.Fatalf("unexpected refreshed job: %+v", updated)
	}

	var knowledge types.Knowledge
	require.NoError(t, db.First(&knowledge, "id = ?", "k-url").Error)
	if knowledge.Title != "旧标题" {
		t.Fatalf("knowledge was updated before review: %+v", knowledge)
	}
}

func TestServiceCreateJobRejectsUnsafeURLBeforePersisting(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	fetcher := &fakeValidatingFetcher{validateErr: errors.New("blocked ip address")}
	svc := NewService(store, fetcher, WithFeatureGate(fakeFeatureGate{enabled: true}))

	_, err := svc.CreateJob(context.Background(), CreateJobInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "http://127.0.0.1/admin",
		Now:         now,
	})
	require.Error(t, err)
	if !strings.Contains(err.Error(), "unsafe source url") {
		t.Fatalf("expected unsafe source url error, got %v", err)
	}
	require.Equal(t, []string{"http://127.0.0.1/admin"}, fetcher.validateCalls)

	var count int64
	require.NoError(t, db.Model(&types.WikaURLRefreshJob{}).Count(&count).Error)
	if count != 0 {
		t.Fatalf("unsafe source url should not persist job, got %d rows", count)
	}
}

func TestServiceReviewApplyUpdatesKnowledgeAndRecordsVersion(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	job, err := store.CreateJob(context.Background(), CreateJobInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		Now:         now,
	})
	require.NoError(t, err)
	_, err = store.AcquireJob(context.Background(), job.ID, "worker-1", now, time.Minute)
	require.NoError(t, err)
	require.NoError(t, store.MarkPendingReview(context.Background(), MarkPendingReviewInput{
		JobID:          job.ID,
		WorkerID:       "worker-1",
		Now:            now,
		FetchedHash:    "hash-new",
		FetchedTitle:   "新标题",
		FetchedContent: "新抓取内容",
	}))

	updater := &fakeKnowledgeUpdater{updated: &types.Knowledge{ID: "k-url", TenantID: 90, KnowledgeBaseID: "kb-url", Title: "新标题", Type: types.KnowledgeTypeManual, EnableStatus: types.ManualKnowledgeStatusPublish}}
	recorder := &fakeVersionRecorder{}
	audit := &fakeURLRefreshAudit{}
	svc := NewService(store, fakeFetcher{}, WithKnowledgeUpdater(updater), WithVersionRecorder(recorder), WithAuditLogger(audit), WithFeatureGate(fakeFeatureGate{enabled: true}))

	result, err := svc.ReviewJob(context.Background(), ReviewJobInput{
		ActorID:  "u-reviewer",
		JobID:    job.ID,
		Decision: ReviewDecisionApply,
		Comment:  "确认更新",
		Now:      now.Add(time.Minute),
	})
	require.NoError(t, err)
	if result.Status != JobStatusApplied || result.VersionID != 700 {
		t.Fatalf("unexpected review result: %+v", result)
	}
	if updater.knowledgeID != "k-url" || updater.payload == nil || updater.payload.Content != "新抓取内容" || updater.payload.Title != "新标题" {
		t.Fatalf("unexpected knowledge update: id=%s payload=%+v", updater.knowledgeID, updater.payload)
	}
	if recorder.input == nil || recorder.input.KnowledgeID != "k-url" || recorder.input.Content != "新抓取内容" || recorder.input.ChangeReason != "url_refresh_apply" {
		t.Fatalf("expected version to be recorded, got %+v", recorder.input)
	}
	if len(audit.entries) != 1 ||
		audit.entries[0].Action != types.AuditActionWikaURLRefreshReviewed ||
		audit.entries[0].TenantID != 90 ||
		audit.entries[0].ActorUserID != "u-reviewer" ||
		audit.entries[0].TargetID != "1" {
		t.Fatalf("unexpected audit entries: %+v", audit.entries)
	}

	var updated types.WikaURLRefreshJob
	require.NoError(t, db.First(&updated, "id = ?", job.ID).Error)
	if updated.Status != JobStatusApplied || updated.ReviewedBy != "u-reviewer" || updated.ReviewComment != "确认更新" {
		t.Fatalf("unexpected reviewed job: %+v", updated)
	}
}

func TestServiceReviewRejectDoesNotUpdateKnowledge(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	job, err := store.CreateJob(context.Background(), CreateJobInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		Now:         now,
	})
	require.NoError(t, err)
	_, err = store.AcquireJob(context.Background(), job.ID, "worker-1", now, time.Minute)
	require.NoError(t, err)
	require.NoError(t, store.MarkPendingReview(context.Background(), MarkPendingReviewInput{
		JobID:          job.ID,
		WorkerID:       "worker-1",
		Now:            now,
		FetchedContent: "新抓取内容",
	}))

	updater := &fakeKnowledgeUpdater{}
	recorder := &fakeVersionRecorder{}
	svc := NewService(store, fakeFetcher{}, WithKnowledgeUpdater(updater), WithVersionRecorder(recorder), WithFeatureGate(fakeFeatureGate{enabled: true}))

	result, err := svc.ReviewJob(context.Background(), ReviewJobInput{
		ActorID:  "u-reviewer",
		JobID:    job.ID,
		Decision: ReviewDecisionReject,
		Comment:  "不采用",
		Now:      now.Add(time.Minute),
	})
	require.NoError(t, err)
	if result.Status != JobStatusRejected || result.VersionID != 0 {
		t.Fatalf("unexpected reject result: %+v", result)
	}
	if updater.payload != nil || recorder.input != nil {
		t.Fatalf("reject should not update knowledge or record version, payload=%+v version=%+v", updater.payload, recorder.input)
	}
}

func TestServiceReviewBeforePendingReviewReturnsStateConflict(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	job, err := store.CreateJob(context.Background(), CreateJobInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		Now:         now,
	})
	require.NoError(t, err)
	svc := NewService(store, fakeFetcher{}, WithKnowledgeUpdater(&fakeKnowledgeUpdater{}), WithVersionRecorder(&fakeVersionRecorder{}), WithFeatureGate(fakeFeatureGate{enabled: true}))

	_, err = svc.ReviewJob(context.Background(), ReviewJobInput{
		ActorID:  "u-reviewer",
		JobID:    job.ID,
		Decision: ReviewDecisionApply,
		Now:      now,
	})
	if err != ErrInvalidJobState {
		t.Fatalf("expected ErrInvalidJobState, got %v", err)
	}
}

func TestServiceFeatureFlagFailClosed(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc := NewService(store, fakeFetcher{}, WithFeatureGate(fakeFeatureGate{enabled: false}))

	_, err := svc.CreateJob(context.Background(), CreateJobInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		Now:         now,
	})
	if err != ErrFeatureDisabled {
		t.Fatalf("expected CreateJob feature disabled, got %v", err)
	}

	job, err := store.CreateJob(context.Background(), CreateJobInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		Now:         now,
	})
	require.NoError(t, err)
	err = svc.RunJob(context.Background(), RunJobInput{JobID: job.ID, WorkerID: "worker-1", Now: now, LeaseDuration: time.Minute})
	if err != ErrFeatureDisabled {
		t.Fatalf("expected RunJob feature disabled, got %v", err)
	}
	var notLeased types.WikaURLRefreshJob
	require.NoError(t, db.First(&notLeased, "id = ?", job.ID).Error)
	if notLeased.Attempts != 0 || notLeased.LockedBy != "" || notLeased.Status != JobStatusPending {
		t.Fatalf("disabled worker should not lease job: %+v", notLeased)
	}
}

func TestServiceCreateOrUpdateScheduleRejectsCronBelowMinimum(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)
	svc := NewService(store, fakeFetcher{}, WithFeatureGate(fakeFeatureGate{enabled: true}))

	_, err := svc.CreateOrUpdateSchedule(context.Background(), CreateOrUpdateScheduleInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		CronExpr:    "*/30 * * * *",
		Enabled:     true,
		Now:         now,
	})
	if err != ErrInvalidSchedule {
		t.Fatalf("expected ErrInvalidSchedule, got %v", err)
	}
}

func TestServiceCreateOrUpdateScheduleCreatesNextRun(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)
	svc := NewService(store, fakeFetcher{}, WithFeatureGate(fakeFeatureGate{enabled: true}))

	schedule, err := svc.CreateOrUpdateSchedule(context.Background(), CreateOrUpdateScheduleInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		CronExpr:    "0 * * * *",
		Enabled:     true,
		Now:         now,
	})
	require.NoError(t, err)
	if schedule.ID == 0 ||
		!schedule.Enabled ||
		schedule.CreatedBy != "u-owner" ||
		schedule.NextRunAt != time.Date(2026, 6, 29, 13, 0, 0, 0, time.UTC) {
		t.Fatalf("unexpected schedule: %+v", schedule)
	}
}

func TestServiceRunDueSchedulesCreatesOneJobPerScheduleSlot(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)
	svc := NewService(store, fakeFetcher{}, WithFeatureGate(fakeFeatureGate{enabled: true}))
	schedule, err := svc.CreateOrUpdateSchedule(context.Background(), CreateOrUpdateScheduleInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		CronExpr:    "0 * * * *",
		Enabled:     true,
		Now:         now,
	})
	require.NoError(t, err)
	require.NoError(t, db.Model(&types.WikaURLRefreshSchedule{}).Where("id = ?", schedule.ID).Update("next_run_at", now.Add(-time.Minute)).Error)

	jobs, err := svc.RunDueSchedules(context.Background(), now)
	require.NoError(t, err)
	if len(jobs) != 1 || jobs[0].ScheduleID == nil || *jobs[0].ScheduleID != schedule.ID {
		t.Fatalf("expected one scheduled job, got %+v", jobs)
	}

	jobs, err = svc.RunDueSchedules(context.Background(), now)
	require.NoError(t, err)
	if len(jobs) != 0 {
		t.Fatalf("expected same due slot to be idempotent after first run, got %+v", jobs)
	}

	var count int64
	require.NoError(t, db.Model(&types.WikaURLRefreshJob{}).Where("schedule_id = ?", schedule.ID).Count(&count).Error)
	if count != 1 {
		t.Fatalf("expected exactly one scheduled job, got %d", count)
	}
}

func TestServiceRunScheduledJobFailureBacksOffAndDisablesWithAudit(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)
	audit := &fakeURLRefreshAudit{}
	svc := NewService(store, fakeFetcher{err: errors.New("upstream timeout")}, WithAuditLogger(audit), WithFeatureGate(fakeFeatureGate{enabled: true}))
	schedule, err := svc.CreateOrUpdateSchedule(context.Background(), CreateOrUpdateScheduleInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		CronExpr:    "0 * * * *",
		Enabled:     true,
		Now:         now,
	})
	require.NoError(t, err)

	failureTimes := make([]time.Time, 0, 3)
	nextFailureTime := now.Add(time.Hour)
	for i := 0; i < 3; i++ {
		failureTime := nextFailureTime
		failureTimes = append(failureTimes, failureTime)
		require.NoError(t, db.Model(&types.WikaURLRefreshSchedule{}).
			Where("id = ?", schedule.ID).
			Update("next_run_at", failureTime).Error)
		jobs, err := svc.RunDueSchedules(context.Background(), failureTime)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		err = svc.RunJob(context.Background(), RunJobInput{
			JobID:         jobs[0].ID,
			WorkerID:      "worker-1",
			Now:           failureTime,
			LeaseDuration: time.Minute,
		})
		require.Error(t, err)
		var afterFailure types.WikaURLRefreshSchedule
		require.NoError(t, db.First(&afterFailure, "id = ?", schedule.ID).Error)
		nextFailureTime = afterFailure.NextRunAt
	}

	var updated types.WikaURLRefreshSchedule
	require.NoError(t, db.First(&updated, "id = ?", schedule.ID).Error)
	if updated.Enabled || updated.ConsecutiveFailures != 3 || updated.LastFailureCode != "fetch_failed" {
		t.Fatalf("expected disabled schedule after three failures, got %+v", updated)
	}
	if updated.NextRunAt.Before(failureTimes[1].Add(6 * time.Hour)) {
		t.Fatalf("expected second failure to back off next run at least 6h, got %s", updated.NextRunAt)
	}
	if len(audit.entries) != 3 ||
		audit.entries[0].Action != types.AuditActionWikaURLRefreshScheduleUpdated ||
		audit.entries[1].Action != types.AuditActionWikaURLRefreshScheduleUpdated ||
		audit.entries[2].Action != types.AuditActionWikaURLRefreshScheduleDisabled {
		t.Fatalf("unexpected audit entries: %+v", audit.entries)
	}
	if audit.entries[2].TargetID != "1" || audit.entries[2].ActorUserID != "u-owner" {
		t.Fatalf("unexpected disabled audit entry: %+v", audit.entries[2])
	}
	disabledDetails := string(audit.entries[2].Details)
	if strings.Contains(disabledDetails, "https://example.com/doc") || strings.Contains(disabledDetails, "upstream timeout") {
		t.Fatalf("schedule audit details should be redacted, got %s", disabledDetails)
	}
}

func TestServiceRunScheduledJobSuccessClearsScheduleFailureState(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)
	svc := NewService(store, fakeFetcher{result: &safefetch.FetchResult{
		Text:        "成功抓取内容",
		ContentType: "text/plain",
		FinalURL:    "https://example.com/doc",
		SizeBytes:   int64(len("成功抓取内容")),
	}}, WithFeatureGate(fakeFeatureGate{enabled: true}))
	schedule, err := svc.CreateOrUpdateSchedule(context.Background(), CreateOrUpdateScheduleInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		CronExpr:    "0 * * * *",
		Enabled:     true,
		Now:         now,
	})
	require.NoError(t, err)
	dueAt := now.Add(time.Hour)
	require.NoError(t, db.Model(&types.WikaURLRefreshSchedule{}).
		Where("id = ?", schedule.ID).
		Updates(map[string]any{
			"next_run_at":          dueAt,
			"consecutive_failures": 2,
			"last_failure_code":    "fetch_failed",
		}).Error)

	jobs, err := svc.RunDueSchedules(context.Background(), dueAt)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.NoError(t, svc.RunJob(context.Background(), RunJobInput{
		JobID:         jobs[0].ID,
		WorkerID:      "worker-1",
		Now:           dueAt,
		LeaseDuration: time.Minute,
	}))

	var updated types.WikaURLRefreshSchedule
	require.NoError(t, db.First(&updated, "id = ?", schedule.ID).Error)
	if !updated.Enabled || updated.ConsecutiveFailures != 0 || updated.LastFailureCode != "" {
		t.Fatalf("expected successful scheduled job to clear failure state, got %+v", updated)
	}
}

func TestServiceUpdateScheduleByID(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)
	svc := NewService(store, fakeFetcher{}, WithFeatureGate(fakeFeatureGate{enabled: true}))
	schedule, err := svc.CreateOrUpdateSchedule(context.Background(), CreateOrUpdateScheduleInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		CronExpr:    "0 * * * *",
		Enabled:     true,
		Now:         now,
	})
	require.NoError(t, err)

	updated, err := svc.UpdateSchedule(context.Background(), UpdateScheduleInput{
		ActorID:    "u-owner",
		ScheduleID: schedule.ID,
		CronExpr:   "0 */2 * * *",
		Enabled:    true,
		Now:        now,
	})
	require.NoError(t, err)
	if !updated.Enabled ||
		updated.CronExpr != "0 */2 * * *" ||
		updated.NextRunAt != time.Date(2026, 6, 29, 14, 0, 0, 0, time.UTC) {
		t.Fatalf("unexpected updated schedule: %+v", updated)
	}
}

func TestServiceDisableSchedule(t *testing.T) {
	db := setupURLRefreshStoreTestDB(t)
	store := NewGormStore(db)
	now := time.Date(2026, 6, 29, 12, 15, 0, 0, time.UTC)
	svc := NewService(store, fakeFetcher{}, WithFeatureGate(fakeFeatureGate{enabled: true}))
	schedule, err := svc.CreateOrUpdateSchedule(context.Background(), CreateOrUpdateScheduleInput{
		ActorID:     "u-owner",
		TenantID:    90,
		KBID:        "kb-url",
		KnowledgeID: "k-url",
		SourceURL:   "https://example.com/doc",
		CronExpr:    "0 * * * *",
		Enabled:     true,
		Now:         now,
	})
	require.NoError(t, err)

	disabled, err := svc.DisableSchedule(context.Background(), DisableScheduleInput{
		ActorID:    "u-owner",
		ScheduleID: schedule.ID,
		Now:        now,
	})
	require.NoError(t, err)
	if disabled.Enabled {
		t.Fatalf("expected disabled schedule, got %+v", disabled)
	}
}
