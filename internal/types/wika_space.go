package types

import "time"

// SpaceType 表示 Wika 在产品层暴露的空间类型。
type SpaceType string

const (
	// SpaceTypePersonal 表示只属于单个用户的个人空间。
	SpaceTypePersonal SpaceType = "personal"
	// SpaceTypeTeam 表示复用原 Tenant 协作语义的团队空间。
	SpaceTypeTeam SpaceType = "team"
)

// IsValid 判断空间类型是否为当前支持的枚举值。
func (s SpaceType) IsValid() bool {
	return s == SpaceTypePersonal || s == SpaceTypeTeam
}

// EnsureSpaceType 将历史或空值租户归一为 team，保持迁移向后兼容。
func (t *Tenant) EnsureSpaceType() {
	if t.SpaceType == "" {
		t.SpaceType = SpaceTypeTeam
	}
}

// UserPersonalSpace 承载“一人一个个人空间”的数据库不变量。
type UserPersonalSpace struct {
	UserID    string    `json:"user_id" gorm:"type:varchar(36);primaryKey"`
	TenantID  uint64    `json:"tenant_id" gorm:"not null;uniqueIndex"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName 绑定表名，避免 GORM 复数化规则变化影响迁移契约。
func (UserPersonalSpace) TableName() string {
	return "user_personal_spaces"
}

// WikaSpaceDefault 保存空间默认知识库，供低摩擦入库入口使用。
type WikaSpaceDefault struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID    uint64    `json:"tenant_id" gorm:"not null;uniqueIndex"`
	DefaultKBID string    `json:"default_kb_id" gorm:"type:varchar(36);not null;index"`
	CreatedBy   string    `json:"created_by" gorm:"type:varchar(36);not null"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 绑定空间默认值表。
func (WikaSpaceDefault) TableName() string {
	return "wika_space_defaults"
}

// WikaSpacePolicy 保存团队空间治理策略。
type WikaSpacePolicy struct {
	ID                uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID          uint64    `json:"tenant_id" gorm:"not null;uniqueIndex"`
	AutoApplyApproved bool      `json:"auto_apply_approved" gorm:"not null;default:false"`
	PolicyVersion     uint64    `json:"policy_version" gorm:"not null;default:1"`
	SafetyPolicy      JSON      `json:"safety_policy" gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TableName 绑定空间策略表。
func (WikaSpacePolicy) TableName() string {
	return "wika_space_policies"
}

// WikaUserToken 保存日常 MCP 工具使用的用户级 PAT 元数据。
type WikaUserToken struct {
	ID          uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID      string     `json:"user_id" gorm:"type:varchar(36);not null;index"`
	TenantID    uint64     `json:"tenant_id" gorm:"not null;index"`
	Name        string     `json:"name" gorm:"type:varchar(128);not null"`
	TokenPrefix string     `json:"token_prefix" gorm:"type:varchar(32);not null"`
	TokenHash   string     `json:"-" gorm:"type:varchar(128);not null;uniqueIndex"`
	HashAlg     string     `json:"hash_alg" gorm:"type:varchar(32);not null;default:'sha256_pepper'"`
	Scopes      JSON       `json:"scopes" gorm:"type:jsonb;not null;default:'[]'"`
	ExpiresAt   time.Time  `json:"expires_at" gorm:"not null;index"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty" gorm:"index"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedByIP string     `json:"created_by_ip,omitempty" gorm:"type:varchar(64)"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// TableName 绑定用户级 MCP token 表。
func (WikaUserToken) TableName() string {
	return "wika_user_tokens"
}
