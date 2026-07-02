package suggestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	wikaversion "github.com/Tencent/WeKnora/internal/wika/governance/version"
)

var (
	ErrSourceKnowledgeNotPersonal = errors.New("source knowledge must belong to personal space")
	ErrSuggestionPermissionDenied = errors.New("suggestion permission denied")
	ErrSuggestionNotApproved      = errors.New("suggestion is not approved")
	ErrSuggestionNotFound         = errors.New("suggestion not found")
	ErrSourceKnowledgeChanged     = errors.New("source knowledge changed since review")
	ErrKnowledgeCreatorMissing    = errors.New("knowledge creator not configured")
	ErrInvalidDecision            = errors.New("invalid suggestion decision")
	ErrTargetDefaultKBNotFound    = errors.New("target space default knowledge base not found")
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
	GetSuggestion(ctx context.Context, suggestionID uint64) (*types.WikaKnowledgeSuggestion, error)
	SaveHumanReview(ctx context.Context, item *types.WikaKnowledgeSuggestion) (*types.WikaKnowledgeSuggestion, error)
	SaveApplyResult(ctx context.Context, suggestionID uint64, resultKnowledgeID, actorID string) (*types.WikaKnowledgeSuggestion, error)
	MarkPendingHuman(ctx context.Context, suggestionID uint64, reason string) error
}

// KnowledgeCreator 复用现有手动知识创建链路复制团队知识。
type KnowledgeCreator interface {
	CreateKnowledgeFromManual(ctx context.Context, kbID string, payload *types.ManualKnowledgePayload, channel string) (*types.Knowledge, error)
}

type VersionRecorder interface {
	RecordVersion(ctx context.Context, input wikaversion.RecordVersionInput) (*types.WikaKnowledgeVersion, error)
}

// Service 编排个人知识推荐到团队的 AI 预审流程。
type Service struct {
	store           Store
	knowledge       KnowledgeCreator
	versionRecorder VersionRecorder
}

type ServiceOption func(*Service)

func WithVersionRecorder(recorder VersionRecorder) ServiceOption {
	return func(s *Service) {
		s.versionRecorder = recorder
	}
}

// NewService 创建 suggestion service。
func NewService(store *GormStore, knowledge KnowledgeCreator, opts ...ServiceOption) *Service {
	svc := &Service{store: store, knowledge: knowledge}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
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
		if strings.TrimSpace(targetKBID) == "" {
			return nil, ErrTargetDefaultKBNotFound
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
		SourceContentHash: sourceContentHash(source.Title, content),
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
	result := resultFromSuggestion(saved, review.Risks)
	if autoApplyEnabled && s.knowledge != nil {
		applyResult, err := s.applyLoadedSuggestion(ctx, input.SubmitterID, saved, false)
		if err != nil {
			return nil, err
		}
		result.AutoApplyResult = applyResult
		result.Status = applyResult.Status
	}
	return result, nil
}

// HumanReview 保存团队维护者对 AI 预审结果的人工覆盖。
func (s *Service) HumanReview(ctx context.Context, input HumanReviewInput) (*SuggestionResult, error) {
	if s.store == nil {
		return nil, errors.New("suggestion store not configured")
	}
	item, err := s.store.GetSuggestion(ctx, input.SuggestionID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrSuggestionNotFound
	}
	if err := s.requireTeamMaintainer(ctx, input.ActorID, item.TargetTenantID); err != nil {
		return nil, err
	}
	status, err := statusForDecision(input.FinalDecision)
	if err != nil {
		return nil, err
	}
	item.HumanDecision = string(input.FinalDecision)
	item.HumanReviewerID = input.ActorID
	item.HumanComment = strings.TrimSpace(input.Comment)
	item.FinalDecision = string(input.FinalDecision)
	item.Status = string(status)
	if title := strings.TrimSpace(input.Title); title != "" {
		item.CorrectedTitle = title
	}
	if content := strings.TrimSpace(input.Content); content != "" {
		item.CorrectedContent = content
	}
	if targetKBID := strings.TrimSpace(input.TargetKBID); targetKBID != "" {
		item.TargetKBID = targetKBID
	}
	if input.Tags != nil {
		item.CorrectedTags = mustJSON(normalizeTags(input.Tags))
	}
	now := time.Now()
	item.ReviewedAt = &now
	saved, err := s.store.SaveHumanReview(ctx, item)
	if err != nil {
		return nil, err
	}
	return resultFromSuggestion(saved, risksFromReview(saved.AIReview)), nil
}

// ApplySuggestion 将已通过的推荐复制到目标团队知识库，并写入 lineage。
func (s *Service) ApplySuggestion(ctx context.Context, input ApplyInput) (*ApplyResult, error) {
	if s.store == nil {
		return nil, errors.New("suggestion store not configured")
	}
	if s.knowledge == nil {
		return nil, ErrKnowledgeCreatorMissing
	}
	item, err := s.store.GetSuggestion(ctx, input.SuggestionID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrSuggestionNotFound
	}
	return s.applyLoadedSuggestion(ctx, input.ActorID, item, true)
}

func (s *Service) applyLoadedSuggestion(ctx context.Context, actorID string, item *types.WikaKnowledgeSuggestion, requireMaintainer bool) (*ApplyResult, error) {
	if item == nil {
		return nil, ErrSuggestionNotFound
	}
	if requireMaintainer {
		if err := s.requireTeamMaintainer(ctx, actorID, item.TargetTenantID); err != nil {
			return nil, err
		}
	}
	if s.knowledge == nil {
		return nil, ErrKnowledgeCreatorMissing
	}
	if Status(item.Status) == StatusApplied && strings.TrimSpace(item.ResultKnowledgeID) != "" {
		return &ApplyResult{ResultKnowledgeID: item.ResultKnowledgeID, Status: StatusApplied}, nil
	}
	if Decision(item.FinalDecision) != DecisionApproved {
		return nil, ErrSuggestionNotApproved
	}
	source, err := s.store.GetKnowledge(ctx, item.SourceKnowledgeID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, ErrSuggestionNotFound
	}
	currentHash := sourceContentHash(source.Title, knowledgeContent(source))
	if strings.TrimSpace(item.SourceContentHash) != "" && item.SourceContentHash != currentHash {
		if markErr := s.store.MarkPendingHuman(ctx, item.ID, "source_content_changed"); markErr != nil {
			return nil, markErr
		}
		return nil, ErrSourceKnowledgeChanged
	}
	title := strings.TrimSpace(item.CorrectedTitle)
	if title == "" {
		title = strings.TrimSpace(source.Title)
	}
	content := strings.TrimSpace(item.CorrectedContent)
	if content == "" {
		content = knowledgeContent(source)
	}
	createCtx := context.WithValue(ctx, types.TenantIDContextKey, item.TargetTenantID)
	created, err := s.knowledge.CreateKnowledgeFromManual(createCtx, item.TargetKBID, &types.ManualKnowledgePayload{
		Title:             title,
		Content:           content,
		Status:            types.ManualKnowledgeStatusPublish,
		Channel:           types.ChannelAPI,
		SkipVersionRecord: s.versionRecorder != nil,
	}, types.ChannelAPI)
	if err != nil {
		return nil, err
	}
	if created == nil || strings.TrimSpace(created.ID) == "" {
		return nil, errors.New("created team knowledge is empty")
	}
	applied, err := s.store.SaveApplyResult(ctx, item.ID, created.ID, actorID)
	if err != nil {
		return nil, err
	}
	if err := s.recordSuggestionApplyVersion(ctx, actorID, applied, created, title, content); err != nil {
		return nil, err
	}
	return &ApplyResult{ResultKnowledgeID: applied.ResultKnowledgeID, Status: Status(applied.Status)}, nil
}

func (s *Service) recordSuggestionApplyVersion(ctx context.Context, actorID string, item *types.WikaKnowledgeSuggestion, created *types.Knowledge, title, content string) error {
	if s.versionRecorder == nil || item == nil || created == nil {
		return nil
	}
	kbID := strings.TrimSpace(created.KnowledgeBaseID)
	if kbID == "" {
		kbID = strings.TrimSpace(item.TargetKBID)
	}
	_, err := s.versionRecorder.RecordVersion(ctx, wikaversion.RecordVersionInput{
		KnowledgeID:  created.ID,
		TenantID:     item.TargetTenantID,
		KBID:         kbID,
		Title:        title,
		Content:      content,
		Tags:         item.CorrectedTags,
		Status:       types.ManualKnowledgeStatusPublish,
		Metadata:     append(types.JSON(nil), created.Metadata...),
		ChangeReason: "suggestion_apply",
		ActorID:      strings.TrimSpace(actorID),
	})
	return err
}

func (s *Service) requireTeamMaintainer(ctx context.Context, actorID string, tenantID uint64) error {
	member, err := s.store.GetTenantMember(ctx, actorID, tenantID)
	if err != nil {
		return err
	}
	if member == nil || member.Status != types.TenantMemberStatusActive {
		return ErrSuggestionPermissionDenied
	}
	if member.Role != types.TenantRoleOwner && member.Role != types.TenantRoleAdmin {
		return ErrSuggestionPermissionDenied
	}
	return nil
}

func statusForDecision(decision Decision) (Status, error) {
	switch decision {
	case DecisionApproved:
		return StatusAIReviewed, nil
	case DecisionNeedsConfirmation:
		return StatusPendingHuman, nil
	case DecisionRejected:
		return StatusRejected, nil
	default:
		return "", ErrInvalidDecision
	}
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
	if len([]rune(reviewableContent(content))) < 12 {
		return reviewResult{Decision: DecisionRejected, Confidence: 0.7, Risks: []string{"content_too_short"}}
	}
	return reviewResult{Decision: DecisionApproved, Confidence: 0.9, Risks: []string{}}
}

func reviewableContent(content string) string {
	body := stripLeadingMarkdownTitle(content)
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if isIntakeAppendixHeading(strings.TrimSpace(line)) {
			return strings.TrimSpace(strings.Join(lines[:i], "\n"))
		}
	}
	return strings.TrimSpace(body)
}

func isIntakeAppendixHeading(line string) bool {
	switch {
	case line == "## 来源", line == "## 证据":
		return true
	case strings.EqualFold(line, "## Source"), strings.EqualFold(line, "## Evidence"):
		return true
	default:
		return false
	}
}

func stripLeadingMarkdownTitle(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "# ") {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= 1 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[1:], "\n"))
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

func sourceContentHash(title, content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(title) + "\n" + strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
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
