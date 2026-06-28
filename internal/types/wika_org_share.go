package types

import "time"

const (
	WikaOrgShareModeReference = "reference"
	WikaOrgShareModeSnapshot  = "snapshot"

	WikaOrgShareStatusPending = "pending"
	WikaOrgShareStatusActive  = "active"
	WikaOrgShareStatusRevoked = "revoked"
)

// WikaOrgShare 表示 P5 Organization 跨团队引用共享授权。
type WikaOrgShare struct {
	ID             uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	OrgID          string     `json:"org_id" gorm:"type:varchar(36);not null;index"`
	SourceTenantID uint64     `json:"source_tenant_id" gorm:"not null;index:idx_wika_org_shares_source_status"`
	SourceKBID     string     `json:"source_kb_id" gorm:"type:varchar(36);not null;index:idx_wika_org_shares_source_status"`
	TargetTenantID uint64     `json:"target_tenant_id" gorm:"not null;index:idx_wika_org_shares_target_status"`
	Mode           string     `json:"mode" gorm:"type:varchar(24);not null;default:'reference'"`
	AllowedFields  JSON       `json:"allowed_fields" gorm:"type:jsonb;not null;default:'[]'"`
	Status         string     `json:"status" gorm:"type:varchar(24);not null;default:'pending';index:idx_wika_org_shares_target_status;index:idx_wika_org_shares_source_status"`
	CreatedBy      string     `json:"created_by" gorm:"type:varchar(64);not null"`
	AcceptedBy     string     `json:"accepted_by,omitempty" gorm:"type:varchar(64)"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	RevokedBy      string     `json:"revoked_by,omitempty" gorm:"type:varchar(64)"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (WikaOrgShare) TableName() string {
	return "wika_org_shares"
}
