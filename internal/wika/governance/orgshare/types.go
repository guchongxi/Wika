package orgshare

import (
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

var ErrFeatureDisabled = errors.New("wika org share feature disabled")
var ErrScopeDenied = errors.New("wika org share scope denied")
var ErrInvalidAllowedFields = errors.New("invalid org share allowed fields")
var ErrShareNotFound = errors.New("org share not found")
var ErrInvalidShareState = errors.New("invalid org share state")

type CreateShareInput struct {
	ActorID        string
	OrgID          string
	SourceTenantID uint64
	SourceKBID     string
	TargetTenantID uint64
	AllowedFields  []string
	Now            time.Time
}

type ListSharesInput struct {
	ActorID  string
	TenantID uint64
	OrgID    string
	Status   string
	Limit    int
	Offset   int
}

type ListSharesResult struct {
	Items []*types.WikaOrgShare `json:"items"`
	Total int64                 `json:"total"`
}

type AcceptShareInput struct {
	ActorID string
	ShareID uint64
	Now     time.Time
}

type RevokeShareInput struct {
	ActorID string
	ShareID uint64
	Now     time.Time
}
