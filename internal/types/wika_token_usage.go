package types

import "time"

// WikaPATUsageContext 是 PAT 鉴权成功后传给调用统计中间件的脱敏元数据。
type WikaPATUsageContext struct {
	TokenID       uint64
	TokenPrefix   string
	UserID        string
	TenantID      uint64
	RequiredScope string
	ToolName      string
	APIMethod     string
	APIPath       string
}

// WikaTokenUsageDaily 保存 Wika PAT 调用的日聚合统计。
type WikaTokenUsageDaily struct {
	ID             uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID       uint64     `json:"tenant_id" gorm:"not null;index;uniqueIndex:ux_wika_token_usage_daily,priority:1"`
	UserID         string     `json:"user_id" gorm:"type:varchar(36);not null;index"`
	TokenID        uint64     `json:"token_id" gorm:"not null;index;uniqueIndex:ux_wika_token_usage_daily,priority:2"`
	ToolName       string     `json:"tool_name" gorm:"type:varchar(64);not null;index;uniqueIndex:ux_wika_token_usage_daily,priority:3"`
	APIMethod      string     `json:"api_method" gorm:"type:varchar(12);not null"`
	APIPath        string     `json:"api_path" gorm:"type:varchar(128);not null"`
	Day            time.Time  `json:"day" gorm:"type:date;not null;index;uniqueIndex:ux_wika_token_usage_daily,priority:4"`
	SuccessCount   int64      `json:"success_count" gorm:"not null;default:0"`
	FailureCount   int64      `json:"failure_count" gorm:"not null;default:0"`
	LastStatusCode int        `json:"last_status_code,omitempty"`
	LastErrorCode  string     `json:"last_error_code,omitempty" gorm:"type:varchar(64)"`
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt  *time.Time `json:"last_failure_at,omitempty"`
	LastLatencyMS  int        `json:"last_latency_ms,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName 绑定 Wika PAT 日聚合统计表。
func (WikaTokenUsageDaily) TableName() string {
	return "wika_token_usage_daily"
}

// WikaTokenUsageEvent 保存 Wika PAT 最近调用事件。
type WikaTokenUsageEvent struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID    uint64    `json:"tenant_id" gorm:"not null;index"`
	UserID      string    `json:"user_id" gorm:"type:varchar(36);not null;index"`
	TokenID     uint64    `json:"token_id" gorm:"not null;index"`
	ToolName    string    `json:"tool_name" gorm:"type:varchar(64);not null;index"`
	APIMethod   string    `json:"api_method" gorm:"type:varchar(12);not null"`
	APIPath     string    `json:"api_path" gorm:"type:varchar(128);not null"`
	StatusCode  int       `json:"status_code" gorm:"not null"`
	Success     bool      `json:"success" gorm:"not null;index"`
	ErrorCode   string    `json:"error_code,omitempty" gorm:"type:varchar(64)"`
	LatencyMS   int       `json:"latency_ms" gorm:"not null;default:0"`
	KnowledgeID string    `json:"knowledge_id,omitempty" gorm:"type:varchar(36);index"`
	CreatedAt   time.Time `json:"created_at" gorm:"index"`
}

// TableName 绑定 Wika PAT 最近调用事件表。
func (WikaTokenUsageEvent) TableName() string {
	return "wika_token_usage_events"
}
