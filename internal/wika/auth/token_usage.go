package auth

import "time"

const (
	// UsageScopeMine 表示只查询当前用户自己的 PAT 统计。
	UsageScopeMine = "mine"
	// UsageScopeAll 表示查询当前租户全部用户的 PAT 统计。
	UsageScopeAll = "all"
)

const activeUsageWindow = 15 * time.Minute

// TokenUsageRecord 是一次 Wika daily API 调用的脱敏统计输入。
type TokenUsageRecord struct {
	TenantID    uint64
	UserID      string
	TokenID     uint64
	ToolName    string
	APIMethod   string
	APIPath     string
	StatusCode  int
	Success     bool
	ErrorCode   string
	LatencyMS   int
	KnowledgeID string
	OccurredAt  time.Time
}

// TokenUsageFilter 是 token 统计和最近事件的查询过滤器。
type TokenUsageFilter struct {
	TenantID     uint64
	ViewerUserID string
	Scope        string
	OwnerUserID  string
	TokenID      uint64
	ToolName     string
	Success      *bool
	From         time.Time
	To           time.Time
	Limit        int
	Offset       int
	Now          time.Time
}

// TokenUsageToken 是统计响应中的 token 元数据，不含明文或 hash。
type TokenUsageToken struct {
	ID               uint64     `json:"id"`
	Name             string     `json:"name"`
	TokenPrefix      string     `json:"token_prefix"`
	OwnerUserID      string     `json:"owner_user_id"`
	OwnerUsername    string     `json:"owner_username,omitempty"`
	OwnerEmail       string     `json:"owner_email,omitempty"`
	Scopes           []string   `json:"scopes"`
	Status           string     `json:"status"`
	ConnectionStatus string     `json:"connection_status"`
	ExpiresAt        time.Time  `json:"expires_at"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at,omitempty"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	TokenHash        string     `json:"-"`
}

// TokenUsageToolSummary 是单个 tool/API 的聚合统计。
type TokenUsageToolSummary struct {
	ToolName       string     `json:"tool_name"`
	APIMethod      string     `json:"api_method"`
	APIPath        string     `json:"api_path"`
	SuccessCount   int64      `json:"success_count"`
	FailureCount   int64      `json:"failure_count"`
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt  *time.Time `json:"last_failure_at,omitempty"`
	LastStatusCode int        `json:"last_status_code,omitempty"`
	LastErrorCode  string     `json:"last_error_code,omitempty"`
	LastLatencyMS  int        `json:"last_latency_ms,omitempty"`
}

// TokenUsageSummary 是单个 token 在查询范围内的聚合统计。
type TokenUsageSummary struct {
	TotalCalls     int64                   `json:"total_calls"`
	SuccessCount   int64                   `json:"success_count"`
	FailureCount   int64                   `json:"failure_count"`
	LastStatusCode int                     `json:"last_status_code,omitempty"`
	LastErrorCode  string                  `json:"last_error_code,omitempty"`
	LastSuccessAt  *time.Time              `json:"last_success_at,omitempty"`
	LastFailureAt  *time.Time              `json:"last_failure_at,omitempty"`
	LastLatencyMS  int                     `json:"last_latency_ms,omitempty"`
	Tools          []TokenUsageToolSummary `json:"tools"`
}

// TokenUsageItem 是统计列表中的一行。
type TokenUsageItem struct {
	Token   TokenUsageToken   `json:"token"`
	Summary TokenUsageSummary `json:"summary"`
}

// TokenUsageListResult 是 token 统计列表响应。
type TokenUsageListResult struct {
	Scope string           `json:"scope"`
	Total int64            `json:"total"`
	Items []TokenUsageItem `json:"items"`
}

// TokenUsageEventItem 是最近调用事件响应中的一行。
type TokenUsageEventItem struct {
	ID            uint64    `json:"id"`
	TokenID       uint64    `json:"token_id"`
	TokenName     string    `json:"token_name"`
	TokenPrefix   string    `json:"token_prefix"`
	OwnerUserID   string    `json:"owner_user_id"`
	OwnerUsername string    `json:"owner_username,omitempty"`
	ToolName      string    `json:"tool_name"`
	APIMethod     string    `json:"api_method"`
	APIPath       string    `json:"api_path"`
	StatusCode    int       `json:"status_code"`
	Success       bool      `json:"success"`
	ErrorCode     string    `json:"error_code,omitempty"`
	LatencyMS     int       `json:"latency_ms"`
	KnowledgeID   string    `json:"knowledge_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// TokenUsageEventListResult 是最近调用事件列表响应。
type TokenUsageEventListResult struct {
	Scope  string                `json:"scope"`
	Total  int64                 `json:"total"`
	Events []TokenUsageEventItem `json:"events"`
}

func normalizeUsageFilter(filter TokenUsageFilter) TokenUsageFilter {
	if filter.Scope == "" {
		filter.Scope = UsageScopeMine
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.Now.IsZero() {
		filter.Now = time.Now()
	}
	return filter
}

func connectionStatus(token TokenUsageToken, summary TokenUsageSummary, now time.Time) string {
	if token.RevokedAt != nil {
		return "revoked"
	}
	if !token.ExpiresAt.IsZero() && !now.Before(token.ExpiresAt) {
		return "expired"
	}
	if token.LastUsedAt == nil && summary.LastSuccessAt == nil && summary.LastFailureAt == nil {
		return "never_used"
	}
	if summary.LastFailureAt != nil && (summary.LastSuccessAt == nil || summary.LastFailureAt.After(*summary.LastSuccessAt)) {
		return "error"
	}
	if summary.LastSuccessAt != nil && now.Sub(*summary.LastSuccessAt) <= activeUsageWindow {
		return "active"
	}
	return "idle"
}

func tokenStatus(token TokenUsageToken, now time.Time) string {
	if token.RevokedAt != nil {
		return "revoked"
	}
	if !token.ExpiresAt.IsZero() && !now.Before(token.ExpiresAt) {
		return "expired"
	}
	return "valid"
}
