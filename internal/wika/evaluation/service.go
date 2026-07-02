package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	wikasearch "github.com/Tencent/WeKnora/internal/wika/search"
)

var (
	ErrEvaluationStoreNotConfigured  = errors.New("evaluation store not configured")
	ErrEvaluationSearchNotConfigured = errors.New("evaluation search runner not configured")
	ErrDatasetNotReadyForFormalRun   = errors.New("dataset has enabled QA without expected knowledge or chunk ids")
	ErrInvalidQAItem                 = errors.New("invalid qa item")
)

// Store 隔离 Wika 评测数据集的数据访问。
type Store interface {
	CreateDataset(ctx context.Context, item *types.WikaEvalDataset) (*types.WikaEvalDataset, error)
	CreateQAItem(ctx context.Context, item *types.WikaEvalQAItem) (*types.WikaEvalQAItem, error)
	ListQAItems(ctx context.Context, datasetID uint64, enabledOnly bool) ([]*types.WikaEvalQAItem, error)
	GetDataset(ctx context.Context, tenantID uint64, kbID string, datasetID uint64) (*types.WikaEvalDataset, error)
	ListRuns(ctx context.Context, tenantID uint64, kbID string, limit int) ([]*types.WikaEvalRun, error)
	CreateRun(ctx context.Context, item *types.WikaEvalRun) (*types.WikaEvalRun, error)
	SaveRunItems(ctx context.Context, items []*types.WikaEvalRunItem) error
	CompleteRun(ctx context.Context, runID uint64, metrics RunMetrics, failed int) (*types.WikaEvalRun, error)
}

// SearchRunner 是正式评测复用 Wika search 的最小接口。
type SearchRunner interface {
	SearchKnowledge(ctx context.Context, input wikasearch.SearchInput) (*wikasearch.SearchResult, error)
}

// Service 负责 Wika 黄金 QA 数据集和正式评测前置校验。
type Service struct {
	store  Store
	search SearchRunner
}

func NewService(store *GormStore, search SearchRunner) *Service {
	return &Service{store: store, search: search}
}

// CreateDataset 创建团队知识库的黄金 QA 数据集。
func (s *Service) CreateDataset(ctx context.Context, input CreateDatasetInput) (*types.WikaEvalDataset, error) {
	if s.store == nil {
		return nil, ErrEvaluationStoreNotConfigured
	}
	now := time.Now()
	return s.store.CreateDataset(ctx, &types.WikaEvalDataset{
		TenantID:    input.TenantID,
		KBID:        strings.TrimSpace(input.KBID),
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		CreatedBy:   strings.TrimSpace(input.ActorID),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

// AddQAItem 新增一条 QA 样例。无 expected IDs 的样例允许保存，但只能 dry-run。
func (s *Service) AddQAItem(ctx context.Context, input AddQAItemInput) (*types.WikaEvalQAItem, error) {
	if s.store == nil {
		return nil, ErrEvaluationStoreNotConfigured
	}
	if input.TenantID != 0 && strings.TrimSpace(input.KBID) != "" {
		if _, err := s.store.GetDataset(ctx, input.TenantID, strings.TrimSpace(input.KBID), input.DatasetID); err != nil {
			return nil, err
		}
	}
	return s.createQAItem(ctx, input)
}

func (s *Service) createQAItem(ctx context.Context, input AddQAItemInput) (*types.WikaEvalQAItem, error) {
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return nil, ErrInvalidQAItem
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	now := time.Now()
	return s.store.CreateQAItem(ctx, &types.WikaEvalQAItem{
		DatasetID:            input.DatasetID,
		Question:             question,
		ExpectedAnswer:       strings.TrimSpace(input.ExpectedAnswer),
		ExpectedKnowledgeIDs: jsonArray(input.ExpectedKnowledgeIDs),
		ExpectedChunkIDs:     jsonArray(input.ExpectedChunkIDs),
		Tags:                 jsonArray(input.Tags),
		Enabled:              enabled,
		Version:              1,
		CreatedAt:            now,
		UpdatedAt:            now,
	})
}

// ImportDataset 批量导入黄金 QA。导入前先校验 dataset 属于当前 tenant/KB。
func (s *Service) ImportDataset(ctx context.Context, input ImportDatasetInput) (*ImportDatasetResult, error) {
	if s.store == nil {
		return nil, ErrEvaluationStoreNotConfigured
	}
	kbID := strings.TrimSpace(input.KBID)
	if _, err := s.store.GetDataset(ctx, input.TenantID, kbID, input.DatasetID); err != nil {
		return nil, err
	}
	for _, item := range input.Items {
		if strings.TrimSpace(item.Question) == "" {
			return nil, ErrInvalidQAItem
		}
	}
	result := &ImportDatasetResult{Items: make([]ImportedQAItem, 0, len(input.Items))}
	for _, item := range input.Items {
		created, err := s.createQAItem(ctx, AddQAItemInput{
			DatasetID:            input.DatasetID,
			Question:             item.Question,
			ExpectedAnswer:       item.ExpectedAnswer,
			ExpectedKnowledgeIDs: item.ExpectedKnowledgeIDs,
			ExpectedChunkIDs:     item.ExpectedChunkIDs,
			Tags:                 item.Tags,
			Enabled:              &item.Enabled,
		})
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, ImportedQAItem{
			ID:       created.ID,
			Question: created.Question,
			Enabled:  created.Enabled,
			Version:  created.Version,
		})
	}
	result.Imported = len(result.Items)
	return result, nil
}

// ValidateDatasetForFormalRun 确保正式 run 的每条启用 QA 都有期望命中 ID。
func (s *Service) ValidateDatasetForFormalRun(ctx context.Context, datasetID uint64) error {
	if s.store == nil {
		return ErrEvaluationStoreNotConfigured
	}
	items, err := s.store.ListQAItems(ctx, datasetID, true)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item == nil || !item.Enabled {
			continue
		}
		if !hasExpectedIDs(item) {
			return ErrDatasetNotReadyForFormalRun
		}
	}
	return nil
}

// DryRun 执行单条 QA 的召回预览，不创建 run，也不写入趋势指标。
func (s *Service) DryRun(ctx context.Context, input DryRunInput) (*DryRunResult, error) {
	if s.search == nil {
		return nil, ErrEvaluationSearchNotConfigured
	}
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return nil, ErrInvalidQAItem
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	searchResult, err := s.search.SearchKnowledge(ctx, wikasearch.SearchInput{
		UserID:      strings.TrimSpace(input.ActorID),
		TenantID:    input.TenantID,
		Query:       question,
		Limit:       limit,
		IncludeTeam: true,
		Format:      "compact",
	})
	if err != nil {
		return nil, err
	}
	if searchResult == nil {
		searchResult = &wikasearch.SearchResult{}
	}
	return &DryRunResult{
		Question:      question,
		Results:       searchResult.Results,
		Truncated:     searchResult.Truncated,
		GraphDegraded: searchResult.GraphDegraded,
	}, nil
}

// ExportDataset 导出黄金 QA 数据集，但不导出 expected_answer。
func (s *Service) ExportDataset(ctx context.Context, input ExportDatasetInput) (*ExportDatasetResult, error) {
	if s.store == nil {
		return nil, ErrEvaluationStoreNotConfigured
	}
	kbID := strings.TrimSpace(input.KBID)
	dataset, err := s.store.GetDataset(ctx, input.TenantID, kbID, input.DatasetID)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListQAItems(ctx, input.DatasetID, false)
	if err != nil {
		return nil, err
	}
	result := &ExportDatasetResult{
		Dataset: ExportDatasetMeta{
			ID:          dataset.ID,
			KBID:        dataset.KBID,
			Name:        dataset.Name,
			Description: dataset.Description,
			CreatedBy:   dataset.CreatedBy,
			CreatedAt:   dataset.CreatedAt,
			UpdatedAt:   dataset.UpdatedAt,
		},
		Items: make([]ExportQAItem, 0, len(items)),
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		result.Items = append(result.Items, ExportQAItem{
			ID:                   item.ID,
			Question:             item.Question,
			ExpectedKnowledgeIDs: stringArray(item.ExpectedKnowledgeIDs),
			ExpectedChunkIDs:     stringArray(item.ExpectedChunkIDs),
			Tags:                 stringArray(item.Tags),
			Enabled:              item.Enabled,
			Version:              item.Version,
			CreatedAt:            item.CreatedAt,
			UpdatedAt:            item.UpdatedAt,
		})
	}
	return result, nil
}

// Trend 返回知识库最近的正式评测 run。
func (s *Service) Trend(ctx context.Context, input TrendInput) (*TrendResult, error) {
	if s.store == nil {
		return nil, ErrEvaluationStoreNotConfigured
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	runs, err := s.store.ListRuns(ctx, input.TenantID, strings.TrimSpace(input.KBID), limit)
	if err != nil {
		return nil, err
	}
	result := &TrendResult{Runs: make([]TrendRun, 0, len(runs))}
	for _, run := range runs {
		if run == nil {
			continue
		}
		result.Runs = append(result.Runs, TrendRun{
			RunID:          run.ID,
			DatasetID:      run.DatasetID,
			DatasetVersion: run.DatasetVersion,
			Status:         run.Status,
			Trigger:        run.Trigger,
			RecallAt5:      run.RecallAt5,
			MRR:            run.MRR,
			NDCGAt5:        run.NDCGAt5,
			Total:          run.Total,
			Failed:         run.Failed,
			CreatedBy:      run.CreatedBy,
			StartedAt:      run.StartedAt,
			CompletedAt:    run.CompletedAt,
			CreatedAt:      run.CreatedAt,
		})
	}
	return result, nil
}

// RunEvaluation 执行一次正式评测 run，并持久化 run 和 case 明细。
func (s *Service) RunEvaluation(ctx context.Context, input RunInput) (*types.WikaEvalRun, error) {
	if s.store == nil {
		return nil, ErrEvaluationStoreNotConfigured
	}
	if s.search == nil {
		return nil, ErrEvaluationSearchNotConfigured
	}
	if _, err := s.store.GetDataset(ctx, input.TenantID, strings.TrimSpace(input.KBID), input.DatasetID); err != nil {
		return nil, err
	}
	if err := s.ValidateDatasetForFormalRun(ctx, input.DatasetID); err != nil {
		return nil, err
	}
	items, err := s.store.ListQAItems(ctx, input.DatasetID, true)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	trigger := RunTriggerManual
	if input.ScheduleID != nil {
		trigger = RunTriggerSchedule
	}
	run, err := s.store.CreateRun(ctx, &types.WikaEvalRun{
		TenantID:       input.TenantID,
		KBID:           strings.TrimSpace(input.KBID),
		DatasetID:      input.DatasetID,
		DatasetVersion: 1,
		ScheduleID:     input.ScheduleID,
		ScheduledFor:   input.ScheduledFor,
		Trigger:        trigger,
		Status:         RunStatusRunning,
		Metrics:        types.JSON([]byte("{}")),
		SearchConfig:   types.JSON([]byte("{}")),
		CreatedBy:      strings.TrimSpace(input.ActorID),
		StartedAt:      &now,
		CreatedAt:      now,
	})
	if err != nil {
		return nil, err
	}

	runItems := make([]*types.WikaEvalRunItem, 0, len(items))
	metrics := RunMetrics{Total: len(items)}
	recallHits := 0
	reciprocalSum := 0.0
	ndcgSum := 0.0
	failed := 0
	for _, item := range items {
		if item == nil {
			continue
		}
		searchResult, err := s.search.SearchKnowledge(ctx, wikasearch.SearchInput{
			UserID:      input.ActorID,
			TenantID:    input.TenantID,
			Query:       item.Question,
			Limit:       5,
			IncludeTeam: true,
			Format:      "compact",
		})
		if err != nil {
			failed++
			runItems = append(runItems, &types.WikaEvalRunItem{
				RunID:         run.ID,
				QAItemID:      item.ID,
				FailureReason: "search_error",
				ErrorMsg:      err.Error(),
				CreatedAt:     time.Now(),
			})
			continue
		}
		retrievedIDs := knowledgeIDs(searchResult)
		firstHitRank := firstExpectedHitRank(retrievedIDs, expectedKnowledgeIDs(item))
		hit := firstHitRank > 0 && firstHitRank <= 5
		if hit {
			recallHits++
			reciprocalSum += 1 / float64(firstHitRank)
			ndcgSum += 1 / math.Log2(float64(firstHitRank+1))
		}
		runItems = append(runItems, &types.WikaEvalRunItem{
			RunID:                 run.ID,
			QAItemID:              item.ID,
			Rank:                  firstHitRank,
			Hit:                   hit,
			FirstHitRank:          firstHitRank,
			RetrievedKnowledgeIDs: jsonArray(retrievedIDs),
			RetrievedChunkIDs:     types.JSON([]byte("[]")),
			CreatedAt:             time.Now(),
		})
	}
	if metrics.Total > 0 {
		metrics.RecallAt5 = float64(recallHits) / float64(metrics.Total)
		metrics.MRR = reciprocalSum / float64(metrics.Total)
		metrics.NDCGAt5 = ndcgSum / float64(metrics.Total)
	}
	if err := s.store.SaveRunItems(ctx, runItems); err != nil {
		return nil, err
	}
	return s.store.CompleteRun(ctx, run.ID, metrics, failed)
}

func hasExpectedIDs(item *types.WikaEvalQAItem) bool {
	return jsonArrayLen(item.ExpectedKnowledgeIDs) > 0 || jsonArrayLen(item.ExpectedChunkIDs) > 0
}

func knowledgeIDs(result *wikasearch.SearchResult) []string {
	if result == nil {
		return nil
	}
	ids := make([]string, 0, len(result.Results))
	for _, item := range result.Results {
		if item.KnowledgeID != "" {
			ids = append(ids, item.KnowledgeID)
		}
	}
	return ids
}

func expectedKnowledgeIDs(item *types.WikaEvalQAItem) []string {
	var ids []string
	_ = json.Unmarshal(item.ExpectedKnowledgeIDs, &ids)
	return ids
}

func firstExpectedHitRank(retrieved, expected []string) int {
	if len(retrieved) == 0 || len(expected) == 0 {
		return 0
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, id := range expected {
		expectedSet[id] = struct{}{}
	}
	for i, id := range retrieved {
		if _, ok := expectedSet[id]; ok {
			return i + 1
		}
	}
	return 0
}

func jsonArray(values []string) types.JSON {
	if len(values) == 0 {
		return types.JSON([]byte("[]"))
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return types.JSON([]byte("[]"))
	}
	return types.JSON(raw)
}

func jsonArrayLen(raw types.JSON) int {
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return 0
	}
	return len(values)
}

func stringArray(raw types.JSON) []string {
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return []string{}
	}
	if values == nil {
		return []string{}
	}
	return values
}

func jsonArrayContains(raw types.JSON, want string) bool {
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return false
	}
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
