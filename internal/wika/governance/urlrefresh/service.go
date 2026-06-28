package urlrefresh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/wika/governance/urlrefresh/safefetch"
)

type Fetcher interface {
	Fetch(ctx context.Context, raw string) (*safefetch.FetchResult, error)
}

type Service struct {
	store   Store
	fetcher Fetcher
}

func NewService(store Store, fetcher Fetcher) *Service {
	return &Service{store: store, fetcher: fetcher}
}

func (s *Service) CreateJob(ctx context.Context, input CreateJobInput) (*types.WikaURLRefreshJob, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.CreateJob(ctx, input)
}

func (s *Service) RunJob(ctx context.Context, input RunJobInput) error {
	if s.store == nil {
		return nil
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	workerID := strings.TrimSpace(input.WorkerID)
	if workerID == "" {
		workerID = "wika-url-refresh-worker"
	}
	job, err := s.store.AcquireJob(ctx, input.JobID, workerID, now, input.LeaseDuration)
	if err != nil {
		return err
	}
	if s.fetcher == nil {
		err := fmt.Errorf("url refresh fetcher unavailable")
		_ = s.store.FailJob(ctx, job.ID, workerID, now, "fetcher_unavailable", err.Error())
		return err
	}
	result, err := s.fetcher.Fetch(ctx, job.SourceURL)
	if err != nil {
		_ = s.store.FailJob(ctx, job.ID, workerID, now, "fetch_failed", err.Error())
		return err
	}
	ssrfCheck, _ := json.Marshal(map[string]any{
		"content_type": result.ContentType,
		"final_url":    result.FinalURL,
		"size_bytes":   result.SizeBytes,
	})
	return s.store.MarkPendingReview(ctx, MarkPendingReviewInput{
		JobID:          job.ID,
		WorkerID:       workerID,
		Now:            now,
		FetchedHash:    hashFetchedContent(result.Text),
		FetchedContent: result.Text,
		DiffSummary:    types.JSON([]byte(`{}`)),
		SSRFCheck:      types.JSON(ssrfCheck),
	})
}

func hashFetchedContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func truncateError(msg string) string {
	msg = strings.TrimSpace(msg)
	if len([]rune(msg)) <= 512 {
		return msg
	}
	runes := []rune(msg)
	return string(runes[:512])
}
