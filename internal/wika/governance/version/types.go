package version

import (
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type RecordVersionInput struct {
	KnowledgeID  string
	TenantID     uint64
	KBID         string
	Title        string
	Content      string
	Tags         types.JSON
	Status       string
	ReviewStatus string
	Metadata     types.JSON
	ContentHash  string
	ChangeReason string
	ActorID      string
	Now          time.Time
}
