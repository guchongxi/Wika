package intake

import "time"

// PushKnowledgeInput 是 Web/MCP 统一知识生产入口的输入。
type PushKnowledgeInput struct {
	UserID         string
	TenantID       uint64
	Title          string
	Content        string
	Source         string
	Tags           []string
	Evidence       string
	ExpiresAt      *time.Time
	IdempotencyKey string
	DryRun         bool
}

// NormalizedKnowledge 是入库前规范化后的知识草稿。
type NormalizedKnowledge struct {
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	Source    string     `json:"source,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	Evidence  string     `json:"evidence,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// DuplicateCandidate 表示疑似重复知识。
type DuplicateCandidate struct {
	KnowledgeID string  `json:"knowledge_id"`
	Title       string  `json:"title"`
	Score       float64 `json:"score"`
	Reason      string  `json:"reason,omitempty"`
}

// PushKnowledgeResult 是 push_knowledge 的统一响应。
type PushKnowledgeResult struct {
	KnowledgeID         string               `json:"knowledge_id,omitempty"`
	TenantID            uint64               `json:"tenant_id,omitempty"`
	KBID                string               `json:"kb_id,omitempty"`
	SourceChannel       string               `json:"source_channel,omitempty"`
	CreatedAt           time.Time            `json:"created_at,omitempty"`
	Normalized          NormalizedKnowledge  `json:"normalized"`
	QualityScore        int                  `json:"quality_score"`
	DuplicateCandidates []DuplicateCandidate `json:"duplicate_candidates"`
	Status              string               `json:"status"`
}
