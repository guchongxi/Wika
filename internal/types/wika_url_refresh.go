package types

import "time"

// WikaURLRefreshJob 表示一次来源 URL 重抓任务。
type WikaURLRefreshJob struct {
	ID             uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ScheduleID     *uint64    `json:"schedule_id,omitempty" gorm:"index;uniqueIndex:ux_wika_url_refresh_jobs_schedule_slot,where:schedule_id IS NOT NULL"`
	KnowledgeID    string     `json:"knowledge_id" gorm:"type:varchar(36);not null;index:idx_wika_url_refresh_jobs_knowledge"`
	TenantID       uint64     `json:"tenant_id" gorm:"not null;index:idx_wika_url_refresh_jobs_knowledge"`
	KBID           string     `json:"kb_id" gorm:"type:varchar(36);not null;index:idx_wika_url_refresh_jobs_knowledge"`
	SourceURL      string     `json:"source_url" gorm:"type:text;not null"`
	ScheduledFor   *time.Time `json:"scheduled_for,omitempty" gorm:"uniqueIndex:ux_wika_url_refresh_jobs_schedule_slot,where:schedule_id IS NOT NULL"`
	IdempotencyKey string     `json:"idempotency_key,omitempty" gorm:"type:varchar(128);index"`
	Status         string     `json:"status" gorm:"type:varchar(24);not null;default:'pending';index:idx_wika_url_refresh_jobs_worker"`
	FetchedHash    string     `json:"fetched_hash,omitempty" gorm:"type:varchar(128)"`
	FetchedTitle   string     `json:"fetched_title,omitempty" gorm:"type:text"`
	FetchedContent string     `json:"fetched_content,omitempty" gorm:"type:text"`
	DiffSummary    JSON       `json:"diff_summary" gorm:"type:jsonb;not null;default:'{}'"`
	SSRFCheck      JSON       `json:"ssrf_check" gorm:"type:jsonb;not null;default:'{}'"`
	FailureCode    string     `json:"failure_code,omitempty" gorm:"type:varchar(64)"`
	ErrorMsg       string     `json:"error_msg,omitempty" gorm:"type:text"`
	Attempts       int        `json:"attempts" gorm:"not null;default:0"`
	LockedUntil    *time.Time `json:"locked_until,omitempty" gorm:"index:idx_wika_url_refresh_jobs_worker"`
	LockedBy       string     `json:"locked_by,omitempty" gorm:"type:varchar(128)"`
	CreatedBy      string     `json:"created_by" gorm:"type:varchar(64);not null"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	ReviewedBy     string     `json:"reviewed_by,omitempty" gorm:"type:varchar(64)"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	ReviewComment  string     `json:"review_comment,omitempty" gorm:"type:text"`
	CreatedAt      time.Time  `json:"created_at" gorm:"index:idx_wika_url_refresh_jobs_knowledge"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (WikaURLRefreshJob) TableName() string {
	return "wika_url_refresh_jobs"
}

// WikaURLRefreshSchedule 表示来源 URL 定时重抓计划。
type WikaURLRefreshSchedule struct {
	ID                  uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID            uint64     `json:"tenant_id" gorm:"not null;index"`
	KBID                string     `json:"kb_id" gorm:"type:varchar(36);not null;index"`
	KnowledgeID         string     `json:"knowledge_id" gorm:"type:varchar(36);not null;uniqueIndex:ux_wika_url_refresh_schedules_enabled,where:enabled = true;index"`
	SourceURL           string     `json:"source_url" gorm:"type:text;not null;uniqueIndex:ux_wika_url_refresh_schedules_enabled,where:enabled = true"`
	Enabled             bool       `json:"enabled" gorm:"not null;default:true;index:idx_wika_url_refresh_schedules_due;uniqueIndex:ux_wika_url_refresh_schedules_enabled,where:enabled = true"`
	CronExpr            string     `json:"cron_expr" gorm:"type:varchar(128);not null"`
	NextRunAt           time.Time  `json:"next_run_at" gorm:"not null;index:idx_wika_url_refresh_schedules_due"`
	LastJobID           *uint64    `json:"last_job_id,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures" gorm:"not null;default:0"`
	LastFailureCode     string     `json:"last_failure_code,omitempty" gorm:"type:varchar(64)"`
	LockedUntil         *time.Time `json:"locked_until,omitempty" gorm:"index:idx_wika_url_refresh_schedules_due"`
	LockedBy            string     `json:"locked_by,omitempty" gorm:"type:varchar(128)"`
	CreatedBy           string     `json:"created_by" gorm:"type:varchar(64);not null"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (WikaURLRefreshSchedule) TableName() string {
	return "wika_url_refresh_schedules"
}
