package suggestion

type Decision string

const (
	DecisionApproved          Decision = "approved"
	DecisionNeedsConfirmation Decision = "needs_confirmation"
	DecisionRejected          Decision = "rejected"
)

type Status string

const (
	StatusAIReviewed   Status = "ai_reviewed"
	StatusPendingHuman Status = "pending_human"
	StatusRejected     Status = "rejected"
	StatusApplied      Status = "applied"
)

// CreateInput 是 suggest_to_team 的输入。
type CreateInput struct {
	SubmitterID    string
	KnowledgeID    string
	TargetTenantID uint64
	TargetKBID     string
	Reason         string
	IdempotencyKey string
}

// HumanReviewInput 是团队维护者人工覆盖 AI 预审的输入。
type HumanReviewInput struct {
	ActorID       string
	SuggestionID  uint64
	FinalDecision Decision
	Title         string
	Content       string
	Tags          []string
	TargetKBID    string
	Comment       string
}

// ApplyInput 是把已通过 suggestion 应用到团队知识库的输入。
type ApplyInput struct {
	ActorID      string
	SuggestionID uint64
}

// SpacePolicy 是团队推荐策略快照。
type SpacePolicy struct {
	AutoApplyApproved bool
	PolicyVersion     int
}

// SuggestionResult 是 suggest_to_team 的响应。
type SuggestionResult struct {
	SuggestionID     uint64       `json:"suggestion_id"`
	AIDecision       Decision     `json:"ai_decision"`
	Status           Status       `json:"status"`
	CorrectedTitle   string       `json:"corrected_title"`
	CorrectedContent string       `json:"corrected_content"`
	Risks            []string     `json:"risks"`
	AutoApplyResult  *ApplyResult `json:"auto_apply_result,omitempty"`
}

// ApplyResult 预留自动应用结果。
type ApplyResult struct {
	ResultKnowledgeID string `json:"result_knowledge_id"`
	Status            Status `json:"status"`
}
