package urlrefresh

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/wika/governance/urlrefresh/safefetch"
	"github.com/stretchr/testify/require"
)

type fakeFetcher struct {
	result *safefetch.FetchResult
	err    error
}

func (f fakeFetcher) Fetch(ctx context.Context, raw string) (*safefetch.FetchResult, error) {
	return f.result, f.err
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
