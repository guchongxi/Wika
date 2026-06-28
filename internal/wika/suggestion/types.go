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
