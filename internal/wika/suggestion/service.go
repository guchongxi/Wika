package suggestion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

var (
	ErrSourceKnowledgeNotPersonal = errors.New("source knowledge must belong to personal space")
	ErrSuggestionPermissionDenied = errors.New("suggestion permission denied")
)

// Store 隔离 suggest_to_team 需要的数据访问。
type Store interface {
	GetKnowledge(ctx context.Context, knowledgeID string) (*types.Knowledge, error)
	GetTenant(ctx context.Context, tenantID uint64) (*types.Tenant, error)
	GetTenantMember(ctx context.Context, userID string, tenantID uint64) (*types.TenantMember, error)
	GetDefaultKB(ctx context.Context, tenantID uint64) (string, error)
	GetSpacePolicy(ctx context.Context, tenantID uint64) (SpacePolicy, error)
	FindByIdempotencyKey(ctx context.Context, submitterID string, targetTenantID uint64, key string) (*SuggestionResult, error)
	SaveSuggestion(ctx context.Context, item *types.WikaKnowledgeSuggestion) (*types.WikaKnowledgeSuggestion, error)
}

// Service 编排个人知识推荐到团队的 AI 预审流程。
type Service struct {
	store Store
}

// NewService 创建 suggestion service。
func NewService(store *GormStore) *Service {
	return &Service{store: store}
}

// CreateSuggestion 创建团队推荐并执行确定性的最小预审。
func (s *Service) CreateSuggestion(ctx context.Context, input CreateInput) (*SuggestionResult, error) {
	if s.store == nil {
		return nil, errors.New("suggestion store not configured")
	}
	if input.IdempotencyKey != "" {
		existing, err := s.store.FindByIdempotencyKey(ctx, input.SubmitterID, input.TargetTenantID, input.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}

	source, err := s.store.GetKnowledge(ctx, input.KnowledgeID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, ErrSuggestionPermissionDenied
	}
	sourceTenant, err := s.store.GetTenant(ctx, source.TenantID)
	if err != nil {
		return nil, err
	}
	if sourceTenant == nil || sourceTenant.SpaceType != types.SpaceTypePersonal {
		return nil, ErrSourceKnowledgeNotPersonal
	}
	sourceMember, err := s.store.GetTenantMember(ctx, input.SubmitterID, source.TenantID)
	if err != nil {
		return nil, err
	}
	if sourceMember == nil || sourceMember.Role != types.TenantRoleOwner || sourceMember.Status != types.TenantMemberStatusActive {
		return nil, ErrSuggestionPermissionDenied
	}
	targetMember, err := s.store.GetTenantMember(ctx, input.SubmitterID, input.TargetTenantID)
	if err != nil {
		return nil, err
	}
	if targetMember == nil || targetMember.Status != types.TenantMemberStatusActive {
		return nil, ErrSuggestionPermissionDenied
	}
	targetKBID := strings.TrimSpace(input.TargetKBID)
	if targetKBID == "" {
		targetKBID, err = s.store.GetDefaultKB(ctx, input.TargetTenantID)
		if err != nil {
			return nil, err
		}
	}
	policy, err := s.store.GetSpacePolicy(ctx, input.TargetTenantID)
	if err != nil {
		return nil, err
	}
	if policy.PolicyVersion <= 0 {
		policy.PolicyVersion = 1
	}

	content := knowledgeContent(source)
	review := reviewKnowledge(source.Title, content)
	status := StatusAIReviewed
	autoApplyEnabled := false
	if review.Decision == DecisionNeedsConfirmation {
		status = StatusPendingHuman
	} else if review.Decision == DecisionRejected {
		status = StatusRejected
	}
	if review.Decision == DecisionApproved && policy.AutoApplyApproved && len(review.Risks) == 0 {
		autoApplyEnabled = true
	}

	now := time.Now()
	item := &types.WikaKnowledgeSuggestion{
		SourceTenantID:    source.TenantID,
		SourceKBID:        source.KnowledgeBaseID,
		SourceKnowledgeID: source.ID,
		TargetTenantID:    input.TargetTenantID,
		TargetKBID:        targetKBID,
		SubmitterID:       input.SubmitterID,
		IdempotencyKey:    strings.TrimSpace(input.IdempotencyKey),
		Reason:            strings.TrimSpace(input.Reason),
		AIDecision:        string(review.Decision),
		AIConfidence:      review.Confidence,
		AIReview:          mustJSON(review),
		CorrectedTitle:    strings.TrimSpace(source.Title),
		CorrectedContent:  content,
		CorrectedTags:     types.JSON([]byte("[]")),
		ChangeSummary:     types.JSON([]byte("[]")),
		FinalDecision:     string(review.Decision),
		Status:            string(status),
		AutoApplyEnabled:  autoApplyEnabled,
		PolicyVersion:     policy.PolicyVersion,
		CreatedAt:         now,
		ReviewedAt:        &now,
	}
	saved, err := s.store.SaveSuggestion(ctx, item)
	if err != nil {
		return nil, err
	}
	return resultFromSuggestion(saved, review.Risks), nil
}

type reviewResult struct {
	Decision   Decision `json:"decision"`
	Confidence float64  `json:"confidence"`
	Risks      []string `json:"risks"`
}

func reviewKnowledge(title, content string) reviewResult {
	text := strings.ToLower(title + "\n" + content)
	risks := make([]string, 0)
	for _, pattern := range []string{"忽略系统指令", "泄露密钥", "ignore system", "api_key", "token="} {
		if strings.Contains(text, strings.ToLower(pattern)) {
			risks = append(risks, pattern)
		}
	}
	if len(risks) > 0 {
		return reviewResult{Decision: DecisionNeedsConfirmation, Confidence: 0.62, Risks: risks}
	}
	if len([]rune(strings.TrimSpace(content))) < 12 {
		return reviewResult{Decision: DecisionRejected, Confidence: 0.7, Risks: []string{"content_too_short"}}
	}
	return reviewResult{Decision: DecisionApproved, Confidence: 0.9, Risks: []string{}}
}

func knowledgeContent(knowledge *types.Knowledge) string {
	if knowledge == nil {
		return ""
	}
	if meta, err := knowledge.ManualMetadata(); err == nil && meta != nil && strings.TrimSpace(meta.Content) != "" {
		return strings.TrimSpace(meta.Content)
	}
	return strings.TrimSpace(knowledge.Description)
}

func resultFromSuggestion(item *types.WikaKnowledgeSuggestion, risks []string) *SuggestionResult {
	if item == nil {
		return nil
	}
	return &SuggestionResult{
		SuggestionID:     item.ID,
		AIDecision:       Decision(item.AIDecision),
		Status:           Status(item.Status),
		CorrectedTitle:   item.CorrectedTitle,
		CorrectedContent: item.CorrectedContent,
		Risks:            risks,
	}
}

func mustJSON(v any) types.JSON {
	b, err := json.Marshal(v)
	if err != nil {
		return types.JSON([]byte("{}"))
	}
	return types.JSON(b)
}
