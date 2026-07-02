package intake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	wikaspace "github.com/Tencent/WeKnora/internal/wika/space"
)

// ErrServiceNotConfigured 表示 intake 服务缺少持久化依赖。
var ErrServiceNotConfigured = errors.New("intake service not configured")

// ErrPersonalDefaultKBNotFound 表示用户还没有个人空间默认知识库。
var ErrPersonalDefaultKBNotFound = errors.New("personal default kb not found")

// DefaultKB 是当前用户个人空间默认知识库。
type DefaultKB struct {
	TenantID uint64
	KBID     string
}

// Store 隔离 intake 需要的 Wika 扩展表读写。
type Store interface {
	GetPersonalDefaultKB(ctx context.Context, userID string) (DefaultKB, error)
	FindKnowledgeIDByIdempotencyKey(ctx context.Context, tenantID uint64, kbID, key string) (string, error)
	SaveKnowledgeState(ctx context.Context, state *types.WikaKnowledgeState) error
}

// KnowledgeCreator 是 intake 复用现有手动知识创建链路的最小接口。
type KnowledgeCreator interface {
	CreateKnowledgeFromManual(ctx context.Context, kbID string, payload *types.ManualKnowledgePayload, channel string) (*types.Knowledge, error)
}

// PersonalSpaceEnsurer 负责在用户首次使用 Wika 时补齐个人空间。
type PersonalSpaceEnsurer interface {
	GetOrCreatePersonalSpace(ctx context.Context, userID, displayName string) (*types.Tenant, error)
}

// Service 编排 Web/MCP 统一知识生产入口。
type Service struct {
	store     Store
	knowledge KnowledgeCreator
	spaces    PersonalSpaceEnsurer
}

// NewService 创建 intake 服务。
func NewService(store *GormStore, knowledge interfaces.KnowledgeService, spaces *wikaspace.Service) *Service {
	return &Service{store: store, knowledge: knowledge, spaces: spaces}
}

// PushKnowledge 提供确定性的规范化、评分、幂等和手动知识入库。
func (s *Service) PushKnowledge(ctx context.Context, input PushKnowledgeInput) (*PushKnowledgeResult, error) {
	normalized := NormalizedKnowledge{
		Title:     strings.TrimSpace(input.Title),
		Content:   strings.TrimSpace(input.Content),
		Source:    strings.TrimSpace(input.Source),
		Tags:      normalizeTags(input.Tags),
		Evidence:  strings.TrimSpace(input.Evidence),
		ExpiresAt: input.ExpiresAt,
	}
	qualityScore := scoreDraft(normalized)
	status := "created"
	if input.DryRun {
		status = "dry_run"
		return &PushKnowledgeResult{
			Normalized:          normalized,
			QualityScore:        qualityScore,
			DuplicateCandidates: []DuplicateCandidate{},
			Status:              status,
		}, nil
	}

	if s.store == nil || s.knowledge == nil {
		return nil, ErrServiceNotConfigured
	}

	defaultKB, err := s.resolvePersonalDefaultKB(ctx, input)
	if err != nil {
		return nil, err
	}
	if input.IdempotencyKey != "" {
		existingID, err := s.store.FindKnowledgeIDByIdempotencyKey(ctx, defaultKB.TenantID, defaultKB.KBID, input.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if existingID != "" {
			return &PushKnowledgeResult{
				KnowledgeID:         existingID,
				TenantID:            defaultKB.TenantID,
				KBID:                defaultKB.KBID,
				SourceChannel:       types.ChannelAPI,
				Normalized:          normalized,
				QualityScore:        qualityScore,
				DuplicateCandidates: []DuplicateCandidate{},
				Status:              "existing",
			}, nil
		}
	}

	payload := &types.ManualKnowledgePayload{
		Title:   normalized.Title,
		Content: buildManualMarkdown(normalized),
		Status:  types.ManualKnowledgeStatusPublish,
		Channel: types.ChannelAPI,
	}
	createCtx := context.WithValue(ctx, types.TenantIDContextKey, defaultKB.TenantID)
	knowledge, err := s.knowledge.CreateKnowledgeFromManual(createCtx, defaultKB.KBID, payload, types.ChannelAPI)
	if err != nil {
		return nil, err
	}
	if knowledge == nil || knowledge.ID == "" {
		return nil, errors.New("created knowledge is empty")
	}

	now := time.Now()
	state := &types.WikaKnowledgeState{
		KnowledgeID:      knowledge.ID,
		TenantID:         defaultKB.TenantID,
		KBID:             defaultKB.KBID,
		QualityScore:     qualityScore,
		QualityBreakdown: types.JSON([]byte("{}")),
		FreshnessStatus:  "fresh",
		ExpiresAt:        normalized.ExpiresAt,
		SourceHash:       sourceHash(normalized),
		IdempotencyKey:   input.IdempotencyKey,
		ReviewStatus:     "none",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.store.SaveKnowledgeState(ctx, state); err != nil {
		return nil, err
	}

	return &PushKnowledgeResult{
		KnowledgeID:         knowledge.ID,
		TenantID:            defaultKB.TenantID,
		KBID:                defaultKB.KBID,
		SourceChannel:       types.ChannelAPI,
		CreatedAt:           now,
		Normalized:          normalized,
		QualityScore:        qualityScore,
		DuplicateCandidates: []DuplicateCandidate{},
		Status:              status,
	}, nil
}

func (s *Service) resolvePersonalDefaultKB(ctx context.Context, input PushKnowledgeInput) (DefaultKB, error) {
	defaultKB, err := s.store.GetPersonalDefaultKB(ctx, input.UserID)
	if err == nil {
		return defaultKB, nil
	}
	if !errors.Is(err, ErrPersonalDefaultKBNotFound) || s.spaces == nil {
		return DefaultKB{}, err
	}
	if _, ensureErr := s.spaces.GetOrCreatePersonalSpace(ctx, input.UserID, ""); ensureErr != nil {
		return DefaultKB{}, ensureErr
	}
	return s.store.GetPersonalDefaultKB(ctx, input.UserID)
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func scoreDraft(draft NormalizedKnowledge) int {
	score := 40
	if draft.Title != "" {
		score += 15
	}
	if len([]rune(draft.Content)) >= 20 {
		score += 20
	}
	if draft.Source != "" || draft.Evidence != "" {
		score += 15
	}
	if len(draft.Tags) > 0 {
		score += 10
	}
	if score > 100 {
		return 100
	}
	return score
}

func buildManualMarkdown(draft NormalizedKnowledge) string {
	var b strings.Builder
	if draft.Title != "" {
		b.WriteString("# ")
		b.WriteString(draft.Title)
		b.WriteString("\n\n")
	}
	b.WriteString(draft.Content)
	if draft.Source != "" {
		b.WriteString("\n\n## 来源\n")
		b.WriteString(draft.Source)
	}
	if draft.Evidence != "" {
		b.WriteString("\n\n## 证据\n")
		b.WriteString(draft.Evidence)
	}
	return b.String()
}

func sourceHash(draft NormalizedKnowledge) string {
	sum := sha256.Sum256([]byte(draft.Title + "\n" + draft.Content + "\n" + draft.Source + "\n" + draft.Evidence))
	return hex.EncodeToString(sum[:])
}
