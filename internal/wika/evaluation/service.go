package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

var (
	ErrEvaluationStoreNotConfigured = errors.New("evaluation store not configured")
	ErrDatasetNotReadyForFormalRun  = errors.New("dataset has enabled QA without expected knowledge or chunk ids")
	ErrInvalidQAItem                = errors.New("invalid qa item")
)

// Store 隔离 Wika 评测数据集的数据访问。
type Store interface {
	CreateDataset(ctx context.Context, item *types.WikaEvalDataset) (*types.WikaEvalDataset, error)
	CreateQAItem(ctx context.Context, item *types.WikaEvalQAItem) (*types.WikaEvalQAItem, error)
	ListQAItems(ctx context.Context, datasetID uint64, enabledOnly bool) ([]*types.WikaEvalQAItem, error)
}

// Service 负责 Wika 黄金 QA 数据集和正式评测前置校验。
type Service struct {
	store Store
}

func NewService(store *GormStore) *Service {
	return &Service{store: store}
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

func hasExpectedIDs(item *types.WikaEvalQAItem) bool {
	return jsonArrayLen(item.ExpectedKnowledgeIDs) > 0 || jsonArrayLen(item.ExpectedChunkIDs) > 0
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
