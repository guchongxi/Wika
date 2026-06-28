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
