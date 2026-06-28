package freshness

import "time"

// RunCheckInput 是触发一次保鲜扫描的输入。
type RunCheckInput struct {
	TenantID uint64
	KBID     string
	Trigger  string
	Now      time.Time
}

const (
	CheckStatusRunning   = "running"
	CheckStatusCompleted = "completed"

	CheckTriggerManual = "manual"

	IssueExpired       = "expired"
	IssueExpiring      = "expiring"
	IssueStale         = "stale"
	IssueLowQuality    = "low_quality"
	IssueLowConfidence = "low_confidence"

	SeverityHigh   = "high"
	SeverityMedium = "medium"

	ItemStatusOpen = "open"
)

const (
	expiringWindow    = 14 * 24 * time.Hour
	staleWindow       = 90 * 24 * time.Hour
	lowQualityScore   = 60
	lowConfidenceRate = 0.6
)
