package evaluation

import (
	"time"

	wikasearch "github.com/Tencent/WeKnora/internal/wika/search"
)

// CreateDatasetInput 是创建黄金 QA 数据集的输入。
type CreateDatasetInput struct {
	ActorID     string
	TenantID    uint64
	KBID        string
	Name        string
	Description string
}

// AddQAItemInput 是新增 QA 样例的输入。
type AddQAItemInput struct {
	ActorID              string
	TenantID             uint64
	KBID                 string
	DatasetID            uint64
	Question             string
	ExpectedAnswer       string
	ExpectedKnowledgeIDs []string
	ExpectedChunkIDs     []string
	Tags                 []string
	Enabled              *bool
}

// ImportDatasetInput 是批量导入黄金 QA 的输入。
type ImportDatasetInput struct {
	ActorID   string
	TenantID  uint64
	KBID      string
	DatasetID uint64
	Items     []ImportQAItem
}

// ImportQAItem 是导入文件中的单条 QA。
type ImportQAItem struct {
	Question             string
	ExpectedAnswer       string
	ExpectedKnowledgeIDs []string
	ExpectedChunkIDs     []string
	Tags                 []string
	Enabled              bool
}

// ImportedQAItem 是导入后返回的条目摘要。
type ImportedQAItem struct {
	ID       uint64 `json:"id"`
	Question string `json:"question"`
	Enabled  bool   `json:"enabled"`
	Version  int    `json:"version"`
}

// ImportDatasetResult 是批量导入结果。
type ImportDatasetResult struct {
	Imported int              `json:"imported"`
	Items    []ImportedQAItem `json:"items"`
}

// RunInput 是触发正式评测 run 的输入。
type RunInput struct {
	ActorID      string
	TenantID     uint64
	KBID         string
	DatasetID    uint64
	ScheduleID   *uint64
	ScheduledFor *time.Time
}

// DryRunInput 是单条 QA dry-run 的输入，不写正式评测指标。
type DryRunInput struct {
	ActorID  string
	TenantID uint64
	KBID     string
	Question string
	Limit    int
}

// ExportDatasetInput 是导出黄金 QA 数据集的输入。
type ExportDatasetInput struct {
	ActorID   string
	TenantID  uint64
	KBID      string
	DatasetID uint64
}

// ExportDatasetMeta 是导出文件中的数据集元信息。
type ExportDatasetMeta struct {
	ID          uint64    `json:"id"`
	KBID        string    `json:"kb_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// ExportQAItem 是可导出的 QA 条目。ExpectedAnswer 只保留空字段，避免泄露标准答案。
type ExportQAItem struct {
	ID                   uint64    `json:"id"`
	Question             string    `json:"question"`
	ExpectedAnswer       string    `json:"expected_answer,omitempty"`
	ExpectedKnowledgeIDs []string  `json:"expected_knowledge_ids"`
	ExpectedChunkIDs     []string  `json:"expected_chunk_ids"`
	Tags                 []string  `json:"tags"`
	Enabled              bool      `json:"enabled"`
	Version              int       `json:"version"`
	CreatedAt            time.Time `json:"created_at,omitempty"`
	UpdatedAt            time.Time `json:"updated_at,omitempty"`
}

// ExportDatasetResult 是黄金 QA 数据集导出结果。
type ExportDatasetResult struct {
	Dataset ExportDatasetMeta `json:"dataset"`
	Items   []ExportQAItem    `json:"items"`
}

// TrendInput 是查询评测趋势的输入。
type TrendInput struct {
	ActorID  string
	TenantID uint64
	KBID     string
	Limit    int
}

// TrendRun 是趋势图中的单次评测 run。
type TrendRun struct {
	RunID          uint64     `json:"run_id"`
	DatasetID      uint64     `json:"dataset_id"`
	DatasetVersion int        `json:"dataset_version"`
	Status         string     `json:"status"`
	Trigger        string     `json:"trigger"`
	RecallAt5      float64    `json:"recall_at_5"`
	MRR            float64    `json:"mrr"`
	NDCGAt5        float64    `json:"ndcg_at_5"`
	Total          int        `json:"total"`
	Failed         int        `json:"failed"`
	CreatedBy      string     `json:"created_by,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at,omitempty"`
}

// TrendResult 是知识库评测趋势结果。
type TrendResult struct {
	Runs []TrendRun `json:"runs"`
}

// DryRunResult 返回单条 QA 的召回预览。
type DryRunResult struct {
	Question      string                  `json:"question"`
	Results       []wikasearch.ResultItem `json:"results"`
	Truncated     bool                    `json:"truncated"`
	GraphDegraded bool                    `json:"graph_degraded"`
}

// RunMetrics 是一次正式评测的核心指标。
type RunMetrics struct {
	Total     int
	RecallAt5 float64
	MRR       float64
	NDCGAt5   float64
}

const (
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunTriggerManual   = "manual"
	RunTriggerSchedule = "schedule"
)
