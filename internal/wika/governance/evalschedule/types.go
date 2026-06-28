package evalschedule

import (
	"errors"
	"time"
)

var ErrInvalidSchedule = errors.New("invalid eval schedule")
var ErrFeatureDisabled = errors.New("wika eval schedule feature disabled")
var ErrScheduleNotFound = errors.New("eval schedule not found")
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
