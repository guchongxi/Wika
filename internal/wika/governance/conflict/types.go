package conflict

import (
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
)

const (
	TriggerManual             = "manual"
	CheckStatusPending        = "pending"
	CheckStatusRunning        = "running"
	CheckStatusCompleted      = "completed"
	CheckStatusFailed         = "failed"
	ItemStatusOpen            = "open"
	ItemStatusConfirmed       = "confirmed"
	ItemStatusDismissed       = "dismissed"
	ItemStatusResolved        = "resolved"
	ConflictTypeContradiction = "contradiction"
	ConflictTypeDuplicate     = "duplicate"
	ConflictTypeOutdated      = "outdated"
	ConflictTypeScopeOverlap  = "scope_overlap"
)

var ErrCheckLeaseUnavailable = errors.New("conflict check lease unavailable")

// Candidate 是 worker 生成的疑似冲突候选。
type Candidate struct {
	SourceKnowledgeID string
	TargetKnowledgeID string
	ConflictType      string
	ConfidenceScore   float64
	Evidence          types.JSON
	AIExplanation     string
}

type GenerateInput struct {
	CheckID  uint64
	TenantID uint64
	KBID     string
	Trigger  string
}
