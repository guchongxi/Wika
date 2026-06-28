package types

import "time"

// WikaKnowledgeState 保存 Wika 入库、质量和保鲜状态的扩展字段。
type WikaKnowledgeState struct {
	KnowledgeID        string     `json:"knowledge_id" gorm:"type:varchar(36);primaryKey"`
	TenantID           uint64     `json:"tenant_id" gorm:"not null;index"`
	KBID               string     `json:"kb_id" gorm:"type:varchar(36);not null;index"`
	QualityScore       int        `json:"quality_score" gorm:"not null;default:0"`
	QualityBreakdown   JSON       `json:"quality_breakdown" gorm:"type:jsonb;not null;default:'{}'"`
	FreshnessStatus    string     `json:"freshness_status" gorm:"type:varchar(24);not null;default:'fresh';index"`
	ConfidenceScore    *float64   `json:"confidence_score,omitempty" gorm:"type:numeric(4,3)"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty" gorm:"index"`
	SourceHash         string     `json:"source_hash,omitempty" gorm:"type:varchar(128)"`
	SourceUpdatedAt    *time.Time `json:"source_updated_at,omitempty"`
	IdempotencyKey     string     `json:"idempotency_key,omitempty" gorm:"type:varchar(128)"`
	LastAccessRollupAt *time.Time `json:"last_access_rollup_at,omitempty"`
	ReviewStatus       string     `json:"review_status" gorm:"type:varchar(24);not null;default:'none'"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// TableName 绑定 Wika 知识状态表。
func (WikaKnowledgeState) TableName() string {
	return "wika_knowledge_state"
}

// KnowledgeAccessDaily 保存知识检索/引用访问的日聚合。
type KnowledgeAccessDaily struct {
	TenantID       uint64    `json:"tenant_id" gorm:"primaryKey;not null"`
	KBID           string    `json:"kb_id" gorm:"type:varchar(36);primaryKey;not null"`
	KnowledgeID    string    `json:"knowledge_id" gorm:"type:varchar(36);primaryKey;not null"`
	Day            time.Time `json:"day" gorm:"type:date;primaryKey;not null"`
	AccessCount    int64     `json:"access_count" gorm:"not null;default:0"`
	LastAccessedAt time.Time `json:"last_accessed_at" gorm:"not null"`
}

// TableName 绑定知识访问日聚合表。
func (KnowledgeAccessDaily) TableName() string {
	return "knowledge_access_daily"
}
