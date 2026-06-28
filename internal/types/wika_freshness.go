package types

import "time"

// WikaFreshnessCheck 表示一次保鲜扫描任务。
type WikaFreshnessCheck struct {
	ID          uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID    uint64     `json:"tenant_id" gorm:"not null;index"`
	KBID        string     `json:"kb_id" gorm:"type:varchar(36);not null;index:idx_freshness_checks_kb_created"`
	Trigger     string     `json:"trigger" gorm:"type:varchar(32);not null"`
	Status      string     `json:"status" gorm:"type:varchar(24);not null;default:'running'"`
	CheckedAt   time.Time  `json:"checked_at" gorm:"not null;index:idx_freshness_checks_kb_created"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (WikaFreshnessCheck) TableName() string {
	return "freshness_checks"
}

// WikaFreshnessCheckItem 表示一次扫描发现的单条保鲜问题。
type WikaFreshnessCheckItem struct {
	ID              uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	CheckID         uint64    `json:"check_id" gorm:"not null;index"`
	TenantID        uint64    `json:"tenant_id" gorm:"not null;index"`
	KBID            string    `json:"kb_id" gorm:"type:varchar(36);not null;index"`
	KnowledgeID     string    `json:"knowledge_id" gorm:"type:varchar(36);not null;index:idx_freshness_check_items_knowledge"`
	IssueType       string    `json:"issue_type" gorm:"type:varchar(32);not null;index:idx_freshness_check_items_status"`
	Severity        string    `json:"severity" gorm:"type:varchar(16);not null"`
	SuggestedAction string    `json:"suggested_action" gorm:"type:varchar(64);not null"`
	Status          string    `json:"status" gorm:"type:varchar(24);not null;default:'open';index:idx_freshness_check_items_status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (WikaFreshnessCheckItem) TableName() string {
	return "freshness_check_items"
}
