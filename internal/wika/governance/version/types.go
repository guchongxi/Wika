package version

import (
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

var ErrVersionNotFound = errors.New("knowledge version not found")

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

type ListVersionsInput struct {
	ActorID     string
	TenantID    uint64
	KnowledgeID string
	SystemAdmin bool
	Limit       int
	Offset      int
}

type GetVersionInput struct {
	ActorID     string
	TenantID    uint64
	KnowledgeID string
	VersionID   uint64
	SystemAdmin bool
}

type DiffInput struct {
	ActorID     string
	TenantID    uint64
	KnowledgeID string
	FromVersion uint64
	ToVersion   uint64
	SystemAdmin bool
}

type DiffResult struct {
	FromVersionNo  int  `json:"from_version_no"`
	ToVersionNo    int  `json:"to_version_no"`
	TitleChanged   bool `json:"title_changed"`
	ContentChanged bool `json:"content_changed"`
	TagsChanged    bool `json:"tags_changed"`
}
