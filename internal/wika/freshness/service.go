package freshness

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	wikaversion "github.com/Tencent/WeKnora/internal/wika/governance/version"
)

var (
	ErrFreshnessStoreNotConfigured = errors.New("freshness store not configured")
	ErrFreshnessItemNotFound       = errors.New("freshness item not found")
	ErrInvalidFreshnessAction      = errors.New("invalid freshness action")
)

// Store 隔离保鲜扫描需要的数据访问。
type Store interface {
	ListKnowledgeStates(ctx context.Context, tenantID uint64, kbID string) ([]*types.WikaKnowledgeState, error)
	LastAccess(ctx context.Context, knowledgeID string) (*types.KnowledgeAccessDaily, error)
	SaveCheck(ctx context.Context, check *types.WikaFreshnessCheck, items []*types.WikaFreshnessCheckItem) (*types.WikaFreshnessCheck, error)
	ListChecks(ctx context.Context, tenantID uint64, kbID string) ([]*types.WikaFreshnessCheck, error)
	ListItems(ctx context.Context, tenantID uint64, kbID, status string) ([]*types.WikaFreshnessCheckItem, error)
	HandleItem(ctx context.Context, update ItemUpdate) (*types.WikaFreshnessCheckItem, error)
}

type AuditLogger interface {
	Log(ctx context.Context, entry *types.AuditLog) error
}

type VersionRecorder interface {
	RecordVersion(ctx context.Context, input wikaversion.RecordVersionInput) (*types.WikaKnowledgeVersion, error)
}

// Service 负责从知识状态和访问聚合生成保鲜问题。
type Service struct {
	store    Store
	audit    AuditLogger
	versions VersionRecorder
}

type ServiceOption func(*Service)

func WithVersionRecorder(recorder VersionRecorder) ServiceOption {
	return func(s *Service) {
		s.versions = recorder
	}
}

func NewService(store *GormStore, audit interfaces.AuditLogService, opts ...ServiceOption) *Service {
	svc := &Service{store: store, audit: audit}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// RunCheck 扫描团队 KB 中过期、将过期、低质、低置信和长期未访问知识。
func (s *Service) RunCheck(ctx context.Context, input RunCheckInput) (*types.WikaFreshnessCheck, error) {
	if s.store == nil {
		return nil, ErrFreshnessStoreNotConfigured
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	trigger := strings.TrimSpace(input.Trigger)
	if trigger == "" {
		trigger = CheckTriggerManual
	}
	states, err := s.store.ListKnowledgeStates(ctx, input.TenantID, strings.TrimSpace(input.KBID))
	if err != nil {
		return nil, err
	}
	items := make([]*types.WikaFreshnessCheckItem, 0)
	for _, state := range states {
		items = append(items, s.itemsForState(ctx, input.TenantID, strings.TrimSpace(input.KBID), state, now)...)
	}
	completedAt := now
	return s.store.SaveCheck(ctx, &types.WikaFreshnessCheck{
		TenantID:    input.TenantID,
		KBID:        strings.TrimSpace(input.KBID),
		Trigger:     trigger,
		Status:      CheckStatusCompleted,
		CheckedAt:   now,
		CompletedAt: &completedAt,
		CreatedAt:   now,
	}, items)
}

func (s *Service) ListChecks(ctx context.Context, input ListInput) ([]*types.WikaFreshnessCheck, error) {
	if s.store == nil {
		return nil, ErrFreshnessStoreNotConfigured
	}
	return s.store.ListChecks(ctx, input.TenantID, strings.TrimSpace(input.KBID))
}

func (s *Service) ListItems(ctx context.Context, input ListInput) ([]*types.WikaFreshnessCheckItem, error) {
	if s.store == nil {
		return nil, ErrFreshnessStoreNotConfigured
	}
	return s.store.ListItems(ctx, input.TenantID, strings.TrimSpace(input.KBID), strings.TrimSpace(input.Status))
}

// Overview 汇总保鲜面板所需的最新扫描和问题数量。
func (s *Service) Overview(ctx context.Context, input ListInput) (*OverviewResult, error) {
	if s.store == nil {
		return nil, ErrFreshnessStoreNotConfigured
	}
	kbID := strings.TrimSpace(input.KBID)
	checks, err := s.store.ListChecks(ctx, input.TenantID, kbID)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListItems(ctx, input.TenantID, kbID, "")
	if err != nil {
		return nil, err
	}
	result := &OverviewResult{
		IssueCounts:     map[string]int{},
		OpenIssueCounts: map[string]int{},
	}
	if len(checks) > 0 {
		result.LatestCheck = checks[0]
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		result.IssueCounts[item.IssueType]++
		switch item.Status {
		case ItemStatusOpen:
			result.TotalOpen++
			result.OpenIssueCounts[item.IssueType]++
		case ItemStatusResolved:
			result.TotalResolved++
		case ItemStatusIgnored:
			result.TotalIgnored++
		}
	}
	return result, nil
}

// HandleItem 处理单条保鲜问题，并写入审计记录。
func (s *Service) HandleItem(ctx context.Context, input HandleItemInput) (*types.WikaFreshnessCheckItem, error) {
	if s.store == nil {
		return nil, ErrFreshnessStoreNotConfigured
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	update, err := itemUpdateFromInput(input, now)
	if err != nil {
		return nil, err
	}
	item, err := s.store.HandleItem(ctx, update)
	if err != nil {
		return nil, err
	}
	if err := s.recordFreshnessHandleVersion(ctx, input, update, item); err != nil {
		return nil, err
	}
	s.emitItemHandledAudit(ctx, input, item)
	return item, nil
}

func (s *Service) itemsForState(ctx context.Context, tenantID uint64, kbID string, state *types.WikaKnowledgeState, now time.Time) []*types.WikaFreshnessCheckItem {
	if state == nil || state.KnowledgeID == "" {
		return nil
	}
	items := make([]*types.WikaFreshnessCheckItem, 0, 2)
	if state.ExpiresAt != nil {
		switch {
		case !state.ExpiresAt.After(now):
			items = append(items, newItem(tenantID, kbID, state.KnowledgeID, IssueExpired, SeverityHigh, "renew_or_deprecate"))
		case state.ExpiresAt.Sub(now) <= expiringWindow:
			items = append(items, newItem(tenantID, kbID, state.KnowledgeID, IssueExpiring, SeverityMedium, "extend_expiry"))
		}
	}
	if state.QualityScore > 0 && state.QualityScore < lowQualityScore {
		items = append(items, newItem(tenantID, kbID, state.KnowledgeID, IssueLowQuality, SeverityHigh, "review_quality"))
	}
	if state.ConfidenceScore != nil && *state.ConfidenceScore < lowConfidenceRate {
		items = append(items, newItem(tenantID, kbID, state.KnowledgeID, IssueLowConfidence, SeverityMedium, "verify_evidence"))
	}
	if access, err := s.store.LastAccess(ctx, state.KnowledgeID); err == nil && access != nil && !access.LastAccessedAt.IsZero() {
		if now.Sub(access.LastAccessedAt) >= staleWindow {
			items = append(items, newItem(tenantID, kbID, state.KnowledgeID, IssueStale, SeverityMedium, "review_usage"))
		}
	}
	return items
}

func itemUpdateFromInput(input HandleItemInput, now time.Time) (ItemUpdate, error) {
	action := strings.TrimSpace(input.Action)
	update := ItemUpdate{
		TenantID: input.TenantID,
		ItemID:   input.ItemID,
		Action:   action,
		Note:     strings.TrimSpace(input.Note),
		ActorID:  strings.TrimSpace(input.ActorID),
		Now:      now,
	}
	switch action {
	case ActionMarkUpdated:
		update.Status = ItemStatusResolved
		update.KnowledgeFreshnessStatus = FreshnessStatusFresh
		update.KnowledgeReviewStatus = ReviewStatusReviewed
	case ActionExtendExpiry:
		if input.ExpiresAt == nil {
			return ItemUpdate{}, ErrInvalidFreshnessAction
		}
		update.Status = ItemStatusResolved
		update.KnowledgeFreshnessStatus = FreshnessStatusFresh
		update.KnowledgeReviewStatus = ReviewStatusReviewed
		update.ExpiresAt = input.ExpiresAt
	case ActionDeprecate:
		update.Status = ItemStatusResolved
		update.KnowledgeFreshnessStatus = FreshnessStatusNeedsReview
		update.KnowledgeReviewStatus = ReviewStatusDeprecated
	case ActionIgnore:
		update.Status = ItemStatusIgnored
	case ActionResuggestToTeam:
		update.Status = ItemStatusResolved
		update.KnowledgeFreshnessStatus = FreshnessStatusNeedsReview
	default:
		return ItemUpdate{}, ErrInvalidFreshnessAction
	}
	return update, nil
}

func (s *Service) emitItemHandledAudit(ctx context.Context, input HandleItemInput, item *types.WikaFreshnessCheckItem) {
	if s.audit == nil || item == nil {
		return
	}
	details, _ := json.Marshal(map[string]any{
		"action":          item.ResolutionAction,
		"previous_status": item.PreviousStatus,
		"status":          item.Status,
		"knowledge_id":    item.KnowledgeID,
		"note":            item.ResolutionNote,
	})
	_ = s.audit.Log(ctx, &types.AuditLog{
		TenantID:    input.TenantID,
		ActorUserID: strings.TrimSpace(input.ActorID),
		Action:      types.AuditActionWikaFreshnessItemHandled,
		TargetType:  "freshness",
		TargetID:    strconv.FormatUint(input.ItemID, 10),
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(details),
	})
}

func (s *Service) recordFreshnessHandleVersion(ctx context.Context, input HandleItemInput, update ItemUpdate, item *types.WikaFreshnessCheckItem) error {
	if s.versions == nil || item == nil {
		return nil
	}
	if update.KnowledgeFreshnessStatus == "" && update.KnowledgeReviewStatus == "" && update.ExpiresAt == nil {
		return nil
	}
	_, err := s.versions.RecordVersion(ctx, wikaversion.RecordVersionInput{
		KnowledgeID:  item.KnowledgeID,
		TenantID:     input.TenantID,
		KBID:         item.KBID,
		Status:       update.KnowledgeFreshnessStatus,
		ReviewStatus: update.KnowledgeReviewStatus,
		ChangeReason: "freshness_handle",
		ActorID:      strings.TrimSpace(input.ActorID),
		Now:          update.Now,
	})
	return err
}

func newItem(tenantID uint64, kbID, knowledgeID, issueType, severity, action string) *types.WikaFreshnessCheckItem {
	return &types.WikaFreshnessCheckItem{
		TenantID:        tenantID,
		KBID:            kbID,
		KnowledgeID:     knowledgeID,
		IssueType:       issueType,
		Severity:        severity,
		SuggestedAction: action,
		Status:          ItemStatusOpen,
	}
}
