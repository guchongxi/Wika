package types

import "time"

// WikaConflictCheck 表示一次冲突检测任务。
type WikaConflictCheck struct {
	ID          uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID    uint64     `json:"tenant_id" gorm:"not null;index:idx_wika_conflict_checks_kb_created"`
	KBID        string     `json:"kb_id" gorm:"type:varchar(36);not null;index:idx_wika_conflict_checks_kb_created"`
	Trigger     string     `json:"trigger" gorm:"type:varchar(32);not null"`
	Status      string     `json:"status" gorm:"type:varchar(24);not null;default:'pending';index:idx_wika_conflict_checks_worker"`
	Attempts    int        `json:"attempts" gorm:"not null;default:0"`
	LockedUntil *time.Time `json:"locked_until,omitempty" gorm:"index:idx_wika_conflict_checks_worker"`
	LockedBy    string     `json:"locked_by,omitempty" gorm:"type:varchar(128)"`
	CreatedBy   string     `json:"created_by" gorm:"type:varchar(64);not null;index"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ErrorMsg    string     `json:"error_msg,omitempty" gorm:"type:text"`
	CreatedAt   time.Time  `json:"created_at" gorm:"index:idx_wika_conflict_checks_kb_created"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (WikaConflictCheck) TableName() string {
	return "wika_conflict_checks"
}

// WikaConflictItem 表示单条疑似冲突候选。
type WikaConflictItem struct {
	ID                uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	CheckID           uint64     `json:"check_id" gorm:"not null;index"`
	TenantID          uint64     `json:"tenant_id" gorm:"not null;index:idx_wika_conflict_items_status;uniqueIndex:ux_wika_conflict_items_open,where:status = 'open' OR status = 'confirmed'"`
	KBID              string     `json:"kb_id" gorm:"type:varchar(36);not null;index:idx_wika_conflict_items_status;uniqueIndex:ux_wika_conflict_items_open,where:status = 'open' OR status = 'confirmed'"`
	SourceKnowledgeID string     `json:"source_knowledge_id" gorm:"type:varchar(36);not null;uniqueIndex:ux_wika_conflict_items_open,where:status = 'open' OR status = 'confirmed'"`
	TargetKnowledgeID string     `json:"target_knowledge_id" gorm:"type:varchar(36);not null;uniqueIndex:ux_wika_conflict_items_open,where:status = 'open' OR status = 'confirmed'"`
	ConflictType      string     `json:"conflict_type" gorm:"type:varchar(32);not null;uniqueIndex:ux_wika_conflict_items_open,where:status = 'open' OR status = 'confirmed'"`
	ConfidenceScore   float64    `json:"confidence_score" gorm:"type:numeric(5,4);not null;default:0"`
	Evidence          JSON       `json:"evidence" gorm:"type:jsonb;not null;default:'{}'"`
	AIExplanation     string     `json:"ai_explanation,omitempty" gorm:"type:text"`
	Status            string     `json:"status" gorm:"type:varchar(24);not null;default:'open';index:idx_wika_conflict_items_status"`
	ReviewerComment   string     `json:"reviewer_comment,omitempty" gorm:"type:text"`
	ResolvedBy        string     `json:"resolved_by,omitempty" gorm:"type:varchar(64)"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" gorm:"index:idx_wika_conflict_items_status"`
}

func (WikaConflictItem) TableName() string {
	return "wika_conflict_items"
}
