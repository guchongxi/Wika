package types

import "time"

// WikaKnowledgeSuggestion 保存个人知识推荐到团队的 AI 预审结果。
type WikaKnowledgeSuggestion struct {
	ID                uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	SourceTenantID    uint64     `json:"source_tenant_id" gorm:"not null;index"`
	SourceKBID        string     `json:"source_kb_id" gorm:"type:varchar(36);not null;index"`
	SourceKnowledgeID string     `json:"source_knowledge_id" gorm:"type:varchar(36);not null;index"`
	TargetTenantID    uint64     `json:"target_tenant_id" gorm:"not null;index"`
	TargetKBID        string     `json:"target_kb_id" gorm:"type:varchar(36);not null;index"`
	SubmitterID       string     `json:"submitter_id" gorm:"type:varchar(64);not null;index"`
	IdempotencyKey    string     `json:"idempotency_key,omitempty" gorm:"type:varchar(128);index"`
	Reason            string     `json:"reason,omitempty" gorm:"type:text"`
	AIDecision        string     `json:"ai_decision" gorm:"type:varchar(32);not null"`
	AIConfidence      float64    `json:"ai_confidence" gorm:"type:numeric(4,3);not null;default:0"`
	AIReview          JSON       `json:"ai_review" gorm:"type:jsonb;not null;default:'{}'"`
	CorrectedTitle    string     `json:"corrected_title" gorm:"type:text"`
	CorrectedContent  string     `json:"corrected_content" gorm:"type:text"`
	CorrectedTags     JSON       `json:"corrected_tags" gorm:"type:jsonb;not null;default:'[]'"`
	ChangeSummary     JSON       `json:"change_summary" gorm:"type:jsonb;not null;default:'[]'"`
	FinalDecision     string     `json:"final_decision" gorm:"type:varchar(32);not null"`
	Status            string     `json:"status" gorm:"type:varchar(32);not null"`
	AutoApplyEnabled  bool       `json:"auto_apply_enabled" gorm:"not null;default:false"`
	PolicyVersion     int        `json:"policy_version" gorm:"not null;default:1"`
	ResultKnowledgeID string     `json:"result_knowledge_id,omitempty" gorm:"type:varchar(36)"`
	CreatedAt         time.Time  `json:"created_at"`
	ReviewedAt        *time.Time `json:"reviewed_at,omitempty"`
	AppliedAt         *time.Time `json:"applied_at,omitempty"`
}

// TableName 绑定 Wika 团队推荐表。
func (WikaKnowledgeSuggestion) TableName() string {
	return "knowledge_suggestions"
}

// WikaKnowledgeLineage 保存知识复制或引用溯源。
type WikaKnowledgeLineage struct {
	ID                uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SourceKnowledgeID string    `json:"source_knowledge_id" gorm:"type:varchar(36);not null;index"`
	TargetKnowledgeID string    `json:"target_knowledge_id" gorm:"type:varchar(36);not null;index"`
	SourceTenantID    uint64    `json:"source_tenant_id" gorm:"not null;index"`
	TargetTenantID    uint64    `json:"target_tenant_id" gorm:"not null;index"`
	Mode              string    `json:"mode" gorm:"type:varchar(24);not null;default:'copy'"`
	SuggestionID      uint64    `json:"suggestion_id" gorm:"not null;index"`
	CreatedBy         string    `json:"created_by" gorm:"type:varchar(64);not null"`
	CreatedAt         time.Time `json:"created_at"`
}

// TableName 绑定 Wika 知识溯源表。
func (WikaKnowledgeLineage) TableName() string {
	return "knowledge_lineage"
}
