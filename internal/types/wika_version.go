package types

import "time"

// WikaKnowledgeVersion 保存知识关键字段的历史快照。
type WikaKnowledgeVersion struct {
	ID           uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	KnowledgeID  string    `json:"knowledge_id" gorm:"type:varchar(36);not null;uniqueIndex:ux_wika_knowledge_versions_no;index:idx_wika_knowledge_versions_list"`
	TenantID     uint64    `json:"tenant_id" gorm:"not null;index:idx_wika_knowledge_versions_list"`
	KBID         string    `json:"kb_id" gorm:"type:varchar(36);not null;index:idx_wika_knowledge_versions_list"`
	VersionNo    int       `json:"version_no" gorm:"not null;uniqueIndex:ux_wika_knowledge_versions_no;index:idx_wika_knowledge_versions_list"`
	Title        string    `json:"title" gorm:"type:text;not null"`
	Content      string    `json:"content" gorm:"type:text;not null"`
	Tags         JSON      `json:"tags" gorm:"type:jsonb;not null;default:'[]'"`
	Status       string    `json:"status" gorm:"type:varchar(32);not null;default:''"`
	ReviewStatus string    `json:"review_status" gorm:"type:varchar(32);not null;default:''"`
	Metadata     JSON      `json:"metadata" gorm:"type:jsonb;not null;default:'{}'"`
	ContentHash  string    `json:"content_hash" gorm:"type:varchar(128);not null"`
	ChangeReason string    `json:"change_reason" gorm:"type:varchar(64);not null;default:''"`
	CreatedBy    string    `json:"created_by" gorm:"type:varchar(64);not null"`
	CreatedAt    time.Time `json:"created_at" gorm:"index:idx_wika_knowledge_versions_list"`
}

func (WikaKnowledgeVersion) TableName() string {
	return "wika_knowledge_versions"
}
