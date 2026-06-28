package search

import "time"

type SourceSpace string

const (
	SourcePersonal SourceSpace = "personal"
	SourceTeam     SourceSpace = "team"
	SourceShared   SourceSpace = "shared"
)

// ReadableScope 表示用户可检索的知识库范围。
type ReadableScope struct {
	TenantID uint64
	KBID     string
	Source   SourceSpace
}

// SearchInput 是 AI 友好的 Wika 知识检索输入。
type SearchInput struct {
	UserID      string
	TenantID    uint64
	Query       string
	Limit       int
	IncludeTeam bool
	Format      string
}

// MineInput 是 get_my_knowledge 的输入。
type MineInput struct {
	UserID string
	Limit  int
	Status string
	Tag    string
}

// AccessRecord 是搜索命中的轻量访问记录。
type AccessRecord struct {
	TenantID    uint64
	KBID        string
	KnowledgeID string
	AccessedAt  time.Time
}

// ResultItem 是默认 compact 检索结果。
type ResultItem struct {
	KnowledgeID     string      `json:"knowledge_id"`
	Title           string      `json:"title"`
	Snippet         string      `json:"snippet"`
	SourceSpace     SourceSpace `json:"source_space"`
	Score           float64     `json:"score"`
	QualityScore    int         `json:"quality_score"`
	FreshnessStatus string      `json:"freshness_status"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// SearchResult 是 search_knowledge 响应。
type SearchResult struct {
	Results   []ResultItem `json:"results"`
	Truncated bool         `json:"truncated"`
}

// MineResult 是 get_my_knowledge 的响应。
type MineResult struct {
	Results []ResultItem `json:"results"`
	Total   int64        `json:"total"`
}
