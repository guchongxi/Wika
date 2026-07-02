package evalschedule

import (
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

var ErrInvalidSchedule = errors.New("invalid eval schedule")
var ErrFeatureDisabled = errors.New("wika eval schedule feature disabled")
var ErrScheduleNotFound = errors.New("eval schedule not found")
var ErrScheduleConflict = errors.New("eval schedule conflict")
var ErrScheduleLeaseUnavailable = errors.New("eval schedule lease unavailable")

type CreateScheduleInput struct {
	ActorID   string
	TenantID  uint64
	KBID      string
	DatasetID uint64
	CronExpr  string
	Enabled   bool
	Now       time.Time
}

type ListInput struct {
	ActorID  string
	TenantID uint64
	KBID     string
	Enabled  *bool
	Limit    int
	Offset   int
}

type ListResult struct {
	Schedules []*types.WikaEvalSchedule `json:"schedules"`
	Total     int64                     `json:"total"`
}

type UpdateScheduleInput struct {
	ActorID    string
	ScheduleID uint64
	CronExpr   string
	Enabled    bool
	Now        time.Time
}

type DisableScheduleInput struct {
	ActorID    string
	ScheduleID uint64
	Now        time.Time
}
