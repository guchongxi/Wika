package freshness

import "time"

// RunCheckInput 是触发一次保鲜扫描的输入。
type RunCheckInput struct {
	TenantID uint64
	KBID     string
	Trigger  string
	Now      time.Time
}

// ListInput 是按 KB 查询保鲜扫描或问题的输入。
type ListInput struct {
	TenantID uint64
	KBID     string
	Status   string
}

// HandleItemInput 是团队维护者处理保鲜问题的输入。
type HandleItemInput struct {
	ActorID   string
	TenantID  uint64
	ItemID    uint64
	Action    string
	Note      string
	ExpiresAt *time.Time
	Now       time.Time
}

// ItemUpdate 是 repository 内部处理保鲜问题的事务参数。
type ItemUpdate struct {
	TenantID                 uint64
	ItemID                   uint64
	Action                   string
	Note                     string
	ActorID                  string
	Status                   string
	KnowledgeFreshnessStatus string
	KnowledgeReviewStatus    string
	ExpiresAt                *time.Time
	Now                      time.Time
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

	ItemStatusOpen     = "open"
	ItemStatusResolved = "resolved"
	ItemStatusIgnored  = "ignored"

	ActionMarkUpdated     = "mark_updated"
	ActionExtendExpiry    = "extend_expiry"
	ActionDeprecate       = "deprecate"
	ActionIgnore          = "ignore"
	ActionResuggestToTeam = "resuggest_to_team"

	FreshnessStatusFresh       = "fresh"
	FreshnessStatusNeedsReview = "needs_review"

	ReviewStatusReviewed   = "reviewed"
	ReviewStatusDeprecated = "deprecated"
)

const (
	expiringWindow    = 14 * 24 * time.Hour
	staleWindow       = 90 * 24 * time.Hour
	lowQualityScore   = 60
	lowConfidenceRate = 0.6
)
