package evaluation

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
	DatasetID            uint64
	Question             string
	ExpectedAnswer       string
	ExpectedKnowledgeIDs []string
	ExpectedChunkIDs     []string
	Tags                 []string
	Enabled              *bool
}

// RunInput 是触发正式评测 run 的输入。
type RunInput struct {
	ActorID   string
	TenantID  uint64
	KBID      string
	DatasetID uint64
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
)
