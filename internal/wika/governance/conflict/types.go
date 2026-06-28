package conflict

import (
	"errors"
	"time"

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
var ErrConflictItemNotFound = errors.New("conflict item not found")
var ErrConflictItemTerminal = errors.New("conflict item is terminal")
var ErrInvalidConflictStatus = errors.New("invalid conflict status")

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

type CreateCheckInput struct {
	ActorID  string
	TenantID uint64
	KBID     string
	Trigger  string
	Now      time.Time
}

type ListItemsInput struct {
	ActorID  string
	TenantID uint64
	KBID     string
	Status   string
	Limit    int
	Offset   int
}

type ResolveItemInput struct {
	ActorID  string
	TenantID uint64
	ItemID   uint64
	Status   string
	Comment  string
	Now      time.Time
}
