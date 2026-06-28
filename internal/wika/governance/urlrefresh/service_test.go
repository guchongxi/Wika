package urlrefresh

import (
	"context"
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
	}})
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
	svc := NewService(store, fakeFetcher{}, WithKnowledgeUpdater(updater), WithVersionRecorder(recorder), WithAuditLogger(audit))

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
	svc := NewService(store, fakeFetcher{}, WithKnowledgeUpdater(updater), WithVersionRecorder(recorder))

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
	svc := NewService(store, fakeFetcher{}, WithKnowledgeUpdater(&fakeKnowledgeUpdater{}), WithVersionRecorder(&fakeVersionRecorder{}))

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
