package types

import "time"

// WikaEvalSchedule 表示 P5 定时评测计划。
type WikaEvalSchedule struct {
	ID                  uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID            uint64     `json:"tenant_id" gorm:"not null;index"`
	KBID                string     `json:"kb_id" gorm:"type:varchar(36);not null;uniqueIndex:ux_wika_eval_schedules_enabled,where:enabled = true;index"`
	DatasetID           uint64     `json:"dataset_id" gorm:"not null;uniqueIndex:ux_wika_eval_schedules_enabled,where:enabled = true;index"`
	Enabled             bool       `json:"enabled" gorm:"not null;default:true;index:idx_wika_eval_schedules_due;uniqueIndex:ux_wika_eval_schedules_enabled,where:enabled = true"`
	CronExpr            string     `json:"cron_expr" gorm:"type:varchar(128);not null"`
	NextRunAt           time.Time  `json:"next_run_at" gorm:"not null;index:idx_wika_eval_schedules_due"`
	LastRunID           *uint64    `json:"last_run_id,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures" gorm:"not null;default:0"`
	LastFailureCode     string     `json:"last_failure_code,omitempty" gorm:"type:varchar(64)"`
	LockedUntil         *time.Time `json:"locked_until,omitempty" gorm:"index:idx_wika_eval_schedules_due"`
	LockedBy            string     `json:"locked_by,omitempty" gorm:"type:varchar(128)"`
	CreatedBy           string     `json:"created_by" gorm:"type:varchar(64);not null"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (WikaEvalSchedule) TableName() string {
	return "wika_eval_schedules"
}
