package types

import "time"

// WikaEvalDataset 保存团队知识库的黄金 QA 数据集。
type WikaEvalDataset struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID    uint64    `json:"tenant_id" gorm:"not null;index"`
	KBID        string    `json:"kb_id" gorm:"type:varchar(36);not null;index"`
	Name        string    `json:"name" gorm:"type:varchar(255);not null"`
	Description string    `json:"description,omitempty" gorm:"type:text"`
	CreatedBy   string    `json:"created_by" gorm:"type:varchar(64);not null;index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (WikaEvalDataset) TableName() string {
	return "eval_datasets"
}

// WikaEvalQAItem 保存评测问题和期望命中。
type WikaEvalQAItem struct {
	ID                   uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	DatasetID            uint64    `json:"dataset_id" gorm:"not null;index:idx_eval_qa_items_dataset_enabled"`
	Question             string    `json:"question" gorm:"type:text;not null"`
	ExpectedAnswer       string    `json:"expected_answer,omitempty" gorm:"type:text"`
	ExpectedKnowledgeIDs JSON      `json:"expected_knowledge_ids" gorm:"type:jsonb;not null;default:'[]'"`
	ExpectedChunkIDs     JSON      `json:"expected_chunk_ids" gorm:"type:jsonb;not null;default:'[]'"`
	Tags                 JSON      `json:"tags" gorm:"type:jsonb;not null;default:'[]'"`
	Enabled              bool      `json:"enabled" gorm:"not null;default:true;index:idx_eval_qa_items_dataset_enabled"`
	Version              int       `json:"version" gorm:"not null;default:1"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (WikaEvalQAItem) TableName() string {
	return "eval_qa_items"
}

// WikaEvalRun 保存一次正式评测 run 的汇总。
type WikaEvalRun struct {
	ID             uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID       uint64     `json:"tenant_id" gorm:"not null;index"`
	KBID           string     `json:"kb_id" gorm:"type:varchar(36);not null;index:idx_eval_runs_kb_created"`
	DatasetID      uint64     `json:"dataset_id" gorm:"not null;index"`
	DatasetVersion int        `json:"dataset_version" gorm:"not null;default:1"`
	Trigger        string     `json:"trigger" gorm:"type:varchar(32);not null"`
	Status         string     `json:"status" gorm:"type:varchar(32);not null"`
	MRR            float64    `json:"mrr" gorm:"column:mrr;type:numeric(8,6);not null;default:0"`
	RecallAt5      float64    `json:"recall_at_5" gorm:"column:recall_at_5;type:numeric(8,6);not null;default:0"`
	NDCGAt5        float64    `json:"ndcg_at_5" gorm:"column:ndcg_at_5;type:numeric(8,6);not null;default:0"`
	Metrics        JSON       `json:"metrics" gorm:"type:jsonb;not null;default:'{}'"`
	SearchConfig   JSON       `json:"search_config" gorm:"type:jsonb;not null;default:'{}'"`
	Total          int        `json:"total" gorm:"not null;default:0"`
	Failed         int        `json:"failed" gorm:"not null;default:0"`
	ErrorMsg       string     `json:"error_msg,omitempty" gorm:"type:text"`
	CreatedBy      string     `json:"created_by" gorm:"type:varchar(64);not null;index"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at" gorm:"index:idx_eval_runs_kb_created"`
}

func (WikaEvalRun) TableName() string {
	return "eval_runs"
}

// WikaEvalRunItem 保存一次 run 中单条 QA 的结果。
type WikaEvalRunItem struct {
	ID                    uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	RunID                 uint64    `json:"run_id" gorm:"not null;index"`
	QAItemID              uint64    `json:"qa_item_id" gorm:"not null;index"`
	Rank                  int       `json:"rank" gorm:"not null;default:0"`
	Score                 float64   `json:"score" gorm:"type:numeric(8,6);not null;default:0"`
	Hit                   bool      `json:"hit" gorm:"not null;default:false"`
	FirstHitRank          int       `json:"first_hit_rank" gorm:"not null;default:0"`
	RetrievedChunkIDs     JSON      `json:"retrieved_chunk_ids" gorm:"type:jsonb;not null;default:'[]'"`
	RetrievedKnowledgeIDs JSON      `json:"retrieved_knowledge_ids" gorm:"type:jsonb;not null;default:'[]'"`
	FailureReason         string    `json:"failure_reason,omitempty" gorm:"type:text"`
	ErrorMsg              string    `json:"error_msg,omitempty" gorm:"type:text"`
	CreatedAt             time.Time `json:"created_at"`
}

func (WikaEvalRunItem) TableName() string {
	return "eval_run_items"
}
