package auth

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormTokenStore 使用 GORM 持久化用户级 MCP token。
type GormTokenStore struct {
	db *gorm.DB
}

// NewGormTokenStore 创建 GORM token 存储。
func NewGormTokenStore(db *gorm.DB) *GormTokenStore {
	return &GormTokenStore{db: db}
}

// SaveToken 保存 token 元数据，不包含明文 token。
func (s *GormTokenStore) SaveToken(ctx context.Context, token *types.WikaUserToken) error {
	return s.db.WithContext(ctx).Create(token).Error
}

// FindTokenByHash 按 hash 读取 token 元数据。
func (s *GormTokenStore) FindTokenByHash(ctx context.Context, tokenHash string) (*types.WikaUserToken, error) {
	var token types.WikaUserToken
	err := s.db.WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenInvalid
		}
		return nil, err
	}
	return &token, nil
}

// ListTokens 列出用户在指定默认空间下创建的 token。
func (s *GormTokenStore) ListTokens(ctx context.Context, userID string, tenantID uint64) ([]*types.WikaUserToken, error) {
	var tokens []*types.WikaUserToken
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Order("created_at DESC, id DESC").
		Find(&tokens).Error
	return tokens, err
}

// RevokeToken 设置 revoked_at；只允许撤销用户自己的 token。
func (s *GormTokenStore) RevokeToken(ctx context.Context, userID string, tokenID uint64, revokedAt time.Time) error {
	res := s.db.WithContext(ctx).
		Model(&types.WikaUserToken{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", tokenID, userID).
		Update("revoked_at", revokedAt)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTokenInvalid
	}
	return nil
}

// RecordUsage 记录一次已通过 Wika PAT 鉴权的 daily API 调用。
func (s *GormTokenStore) RecordUsage(ctx context.Context, record TokenUsageRecord) error {
	if record.OccurredAt.IsZero() {
		record.OccurredAt = time.Now()
	}
	if record.LatencyMS < 0 {
		record.LatencyMS = 0
	}
	day := usageDay(record.OccurredAt)
	successCount := int64(0)
	failureCount := int64(0)
	var lastSuccessAt *time.Time
	var lastFailureAt *time.Time
	if record.Success {
		successCount = 1
		lastSuccessAt = &record.OccurredAt
	} else {
		failureCount = 1
		lastFailureAt = &record.OccurredAt
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&types.WikaUserToken{}).
			Where("id = ? AND tenant_id = ? AND user_id = ?", record.TokenID, record.TenantID, record.UserID).
			Update("last_used_at", record.OccurredAt)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrTokenInvalid
		}

		if err := tx.Create(&types.WikaTokenUsageEvent{
			TenantID:    record.TenantID,
			UserID:      record.UserID,
			TokenID:     record.TokenID,
			ToolName:    record.ToolName,
			APIMethod:   record.APIMethod,
			APIPath:     record.APIPath,
			StatusCode:  record.StatusCode,
			Success:     record.Success,
			ErrorCode:   record.ErrorCode,
			LatencyMS:   record.LatencyMS,
			KnowledgeID: record.KnowledgeID,
			CreatedAt:   record.OccurredAt,
		}).Error; err != nil {
			return err
		}

		daily := &types.WikaTokenUsageDaily{
			TenantID:       record.TenantID,
			UserID:         record.UserID,
			TokenID:        record.TokenID,
			ToolName:       record.ToolName,
			APIMethod:      record.APIMethod,
			APIPath:        record.APIPath,
			Day:            day,
			SuccessCount:   successCount,
			FailureCount:   failureCount,
			LastStatusCode: record.StatusCode,
			LastErrorCode:  record.ErrorCode,
			LastSuccessAt:  lastSuccessAt,
			LastFailureAt:  lastFailureAt,
			LastLatencyMS:  record.LatencyMS,
			CreatedAt:      record.OccurredAt,
			UpdatedAt:      record.OccurredAt,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tenant_id"},
				{Name: "token_id"},
				{Name: "tool_name"},
				{Name: "day"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"success_count":    gorm.Expr("wika_token_usage_daily.success_count + excluded.success_count"),
				"failure_count":    gorm.Expr("wika_token_usage_daily.failure_count + excluded.failure_count"),
				"last_status_code": gorm.Expr("excluded.last_status_code"),
				"last_error_code":  gorm.Expr("excluded.last_error_code"),
				"last_success_at":  gorm.Expr("CASE WHEN excluded.last_success_at IS NOT NULL THEN excluded.last_success_at ELSE wika_token_usage_daily.last_success_at END"),
				"last_failure_at":  gorm.Expr("CASE WHEN excluded.last_failure_at IS NOT NULL THEN excluded.last_failure_at ELSE wika_token_usage_daily.last_failure_at END"),
				"last_latency_ms":  gorm.Expr("excluded.last_latency_ms"),
				"updated_at":       gorm.Expr("excluded.updated_at"),
			}),
		}).Create(daily).Error
	})
}

// ListUsage 返回当前过滤范围内的 token 调用聚合。
func (s *GormTokenStore) ListUsage(ctx context.Context, filter TokenUsageFilter) (*TokenUsageListResult, error) {
	filter = normalizeUsageFilter(filter)
	tokens, err := s.listUsageTokens(ctx, filter)
	if err != nil {
		return nil, err
	}
	userMap, err := s.loadUsageUsers(ctx, tokens)
	if err != nil {
		return nil, err
	}

	items := make([]TokenUsageItem, 0, len(tokens))
	for _, token := range tokens {
		summary, err := s.buildTokenUsageSummary(ctx, token.ID, filter)
		if err != nil {
			return nil, err
		}
		if filter.ToolName != "" && len(summary.Tools) == 0 {
			continue
		}
		if filter.Success != nil {
			if *filter.Success && summary.SuccessCount == 0 {
				continue
			}
			if !*filter.Success && summary.FailureCount == 0 {
				continue
			}
		}
		owner := userMap[token.UserID]
		respToken := buildUsageToken(token, owner, filter.Now)
		respToken.ConnectionStatus = connectionStatus(respToken, summary, filter.Now)
		items = append(items, TokenUsageItem{Token: respToken, Summary: summary})
	}

	total := int64(len(items))
	start := filter.Offset
	if start > len(items) {
		start = len(items)
	}
	end := start + filter.Limit
	if end > len(items) {
		end = len(items)
	}
	return &TokenUsageListResult{
		Scope: filter.Scope,
		Total: total,
		Items: items[start:end],
	}, nil
}

// ListUsageEvents 返回当前过滤范围内的最近调用事件。
func (s *GormTokenStore) ListUsageEvents(ctx context.Context, filter TokenUsageFilter) (*TokenUsageEventListResult, error) {
	filter = normalizeUsageFilter(filter)
	if filter.Limit > 500 {
		filter.Limit = 500
	}
	var events []*types.WikaTokenUsageEvent
	q := s.db.WithContext(ctx).
		Model(&types.WikaTokenUsageEvent{}).
		Where("tenant_id = ?", filter.TenantID)
	q = applyUsageEventFilter(q, filter)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	if err := q.Order("created_at DESC, id DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&events).Error; err != nil {
		return nil, err
	}

	tokenMap, err := s.loadTokensByID(ctx, usageEventTokenIDs(events))
	if err != nil {
		return nil, err
	}
	userMap, err := s.loadUsersByID(ctx, usageEventUserIDs(events))
	if err != nil {
		return nil, err
	}

	out := make([]TokenUsageEventItem, 0, len(events))
	for _, event := range events {
		token := tokenMap[event.TokenID]
		owner := userMap[event.UserID]
		out = append(out, TokenUsageEventItem{
			ID:            event.ID,
			TokenID:       event.TokenID,
			TokenName:     token.Name,
			TokenPrefix:   token.TokenPrefix,
			OwnerUserID:   event.UserID,
			OwnerUsername: owner.Username,
			ToolName:      event.ToolName,
			APIMethod:     event.APIMethod,
			APIPath:       event.APIPath,
			StatusCode:    event.StatusCode,
			Success:       event.Success,
			ErrorCode:     event.ErrorCode,
			LatencyMS:     event.LatencyMS,
			KnowledgeID:   event.KnowledgeID,
			CreatedAt:     event.CreatedAt,
		})
	}
	return &TokenUsageEventListResult{Scope: filter.Scope, Total: total, Events: out}, nil
}

func usageDay(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *GormTokenStore) listUsageTokens(ctx context.Context, filter TokenUsageFilter) ([]*types.WikaUserToken, error) {
	var tokens []*types.WikaUserToken
	q := s.db.WithContext(ctx).
		Where("tenant_id = ?", filter.TenantID).
		Order("created_at DESC, id DESC")
	if filter.Scope == UsageScopeMine {
		q = q.Where("user_id = ?", filter.ViewerUserID)
	}
	if filter.OwnerUserID != "" {
		q = q.Where("user_id = ?", filter.OwnerUserID)
	}
	if filter.TokenID != 0 {
		q = q.Where("id = ?", filter.TokenID)
	}
	err := q.Find(&tokens).Error
	return tokens, err
}

func (s *GormTokenStore) buildTokenUsageSummary(ctx context.Context, tokenID uint64, filter TokenUsageFilter) (TokenUsageSummary, error) {
	var rows []*types.WikaTokenUsageDaily
	q := s.db.WithContext(ctx).
		Where("tenant_id = ? AND token_id = ?", filter.TenantID, tokenID)
	if filter.ToolName != "" {
		q = q.Where("tool_name = ?", filter.ToolName)
	}
	if !filter.From.IsZero() {
		q = q.Where("day >= ?", usageDay(filter.From))
	}
	if !filter.To.IsZero() {
		q = q.Where("day <= ?", usageDay(filter.To))
	}
	if err := q.Find(&rows).Error; err != nil {
		return TokenUsageSummary{}, err
	}

	byTool := map[string]*TokenUsageToolSummary{}
	var summary TokenUsageSummary
	for _, row := range rows {
		key := row.ToolName + "\x00" + row.APIMethod + "\x00" + row.APIPath
		tool := byTool[key]
		if tool == nil {
			tool = &TokenUsageToolSummary{ToolName: row.ToolName, APIMethod: row.APIMethod, APIPath: row.APIPath}
			byTool[key] = tool
		}
		tool.SuccessCount += row.SuccessCount
		tool.FailureCount += row.FailureCount
		summary.SuccessCount += row.SuccessCount
		summary.FailureCount += row.FailureCount
		mergeLastUsage(&summary, row.LastStatusCode, row.LastErrorCode, row.LastSuccessAt, row.LastFailureAt, row.LastLatencyMS)
		mergeLastUsageTool(tool, row)
	}
	summary.TotalCalls = summary.SuccessCount + summary.FailureCount
	for _, tool := range byTool {
		summary.Tools = append(summary.Tools, *tool)
	}
	return summary, nil
}

func mergeLastUsage(summary *TokenUsageSummary, statusCode int, errorCode string, successAt, failureAt *time.Time, latencyMS int) {
	if successAt != nil && (summary.LastSuccessAt == nil || successAt.After(*summary.LastSuccessAt)) {
		summary.LastSuccessAt = successAt
	}
	if failureAt != nil && (summary.LastFailureAt == nil || failureAt.After(*summary.LastFailureAt)) {
		summary.LastFailureAt = failureAt
	}
	lastAt := summary.LastSuccessAt
	if summary.LastFailureAt != nil && (lastAt == nil || summary.LastFailureAt.After(*lastAt)) {
		lastAt = summary.LastFailureAt
	}
	if lastAt == successAt || lastAt == failureAt {
		summary.LastStatusCode = statusCode
		summary.LastErrorCode = errorCode
		summary.LastLatencyMS = latencyMS
	}
}

func mergeLastUsageTool(tool *TokenUsageToolSummary, row *types.WikaTokenUsageDaily) {
	if row.LastSuccessAt != nil && (tool.LastSuccessAt == nil || row.LastSuccessAt.After(*tool.LastSuccessAt)) {
		tool.LastSuccessAt = row.LastSuccessAt
	}
	if row.LastFailureAt != nil && (tool.LastFailureAt == nil || row.LastFailureAt.After(*tool.LastFailureAt)) {
		tool.LastFailureAt = row.LastFailureAt
	}
	tool.LastStatusCode = row.LastStatusCode
	tool.LastErrorCode = row.LastErrorCode
	tool.LastLatencyMS = row.LastLatencyMS
}

func buildUsageToken(token *types.WikaUserToken, owner *types.User, now time.Time) TokenUsageToken {
	out := TokenUsageToken{
		ID:               token.ID,
		Name:             token.Name,
		TokenPrefix:      token.TokenPrefix,
		OwnerUserID:      token.UserID,
		Scopes:           scopesFromJSON(token.Scopes),
		ExpiresAt:        token.ExpiresAt,
		RevokedAt:        token.RevokedAt,
		CreatedAt:        token.CreatedAt,
		LastUsedAt:       token.LastUsedAt,
		ConnectionStatus: "never_used",
	}
	out.Status = tokenStatus(out, now)
	if owner != nil {
		out.OwnerUsername = owner.Username
		out.OwnerEmail = owner.Email
	}
	return out
}

func scopesFromJSON(raw types.JSON) []string {
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return []string{}
	}
	return scopes
}

func (s *GormTokenStore) loadUsageUsers(ctx context.Context, tokens []*types.WikaUserToken) (map[string]*types.User, error) {
	return s.loadUsersByID(ctx, usageTokenUserIDs(tokens))
}

func (s *GormTokenStore) loadUsersByID(ctx context.Context, ids []string) (map[string]*types.User, error) {
	out := map[string]*types.User{}
	if len(ids) == 0 {
		return out, nil
	}
	var users []*types.User
	if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		out[user.ID] = user
	}
	return out, nil
}

func (s *GormTokenStore) loadTokensByID(ctx context.Context, ids []uint64) (map[uint64]*types.WikaUserToken, error) {
	out := map[uint64]*types.WikaUserToken{}
	if len(ids) == 0 {
		return out, nil
	}
	var tokens []*types.WikaUserToken
	if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&tokens).Error; err != nil {
		return nil, err
	}
	for _, token := range tokens {
		out[token.ID] = token
	}
	return out, nil
}

func usageTokenUserIDs(tokens []*types.WikaUserToken) []string {
	seen := map[string]struct{}{}
	var ids []string
	for _, token := range tokens {
		if token.UserID == "" {
			continue
		}
		if _, ok := seen[token.UserID]; ok {
			continue
		}
		seen[token.UserID] = struct{}{}
		ids = append(ids, token.UserID)
	}
	return ids
}

func usageEventUserIDs(events []*types.WikaTokenUsageEvent) []string {
	seen := map[string]struct{}{}
	var ids []string
	for _, event := range events {
		if event.UserID == "" {
			continue
		}
		if _, ok := seen[event.UserID]; ok {
			continue
		}
		seen[event.UserID] = struct{}{}
		ids = append(ids, event.UserID)
	}
	return ids
}

func usageEventTokenIDs(events []*types.WikaTokenUsageEvent) []uint64 {
	seen := map[uint64]struct{}{}
	var ids []uint64
	for _, event := range events {
		if event.TokenID == 0 {
			continue
		}
		if _, ok := seen[event.TokenID]; ok {
			continue
		}
		seen[event.TokenID] = struct{}{}
		ids = append(ids, event.TokenID)
	}
	return ids
}

func applyUsageEventFilter(q *gorm.DB, filter TokenUsageFilter) *gorm.DB {
	if filter.Scope == UsageScopeMine {
		q = q.Where("user_id = ?", filter.ViewerUserID)
	}
	if filter.OwnerUserID != "" {
		q = q.Where("user_id = ?", filter.OwnerUserID)
	}
	if filter.TokenID != 0 {
		q = q.Where("token_id = ?", filter.TokenID)
	}
	if filter.ToolName != "" {
		q = q.Where("tool_name = ?", filter.ToolName)
	}
	if filter.Success != nil {
		q = q.Where("success = ?", *filter.Success)
	}
	if !filter.From.IsZero() {
		q = q.Where("created_at >= ?", usageDay(filter.From))
	}
	if !filter.To.IsZero() {
		q = q.Where("created_at < ?", usageDay(filter.To).AddDate(0, 0, 1))
	}
	return q
}
