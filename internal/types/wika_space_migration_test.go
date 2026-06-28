package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWikaSpaceMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "versioned", "000090_wika_space.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Wika space migration to exist: %v", err)
	}

	sql := string(raw)
	required := []string{
		"ADD COLUMN IF NOT EXISTS space_type",
		"CHECK (space_type IN ('personal', 'team'))",
		"CREATE TABLE IF NOT EXISTS user_personal_spaces",
		"tenant_id BIGINT NOT NULL UNIQUE",
		"REFERENCES tenants(id)",
		"idx_tenants_space_type",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}

func TestWikaP1aGovernanceMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "versioned", "000091_wika_space_defaults_tokens.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Wika P1a governance migration to exist: %v", err)
	}

	sql := string(raw)
	required := []string{
		"CREATE TABLE IF NOT EXISTS wika_space_defaults",
		"CREATE TABLE IF NOT EXISTS wika_space_policies",
		"CREATE TABLE IF NOT EXISTS wika_user_tokens",
		"default_kb_id VARCHAR(36) NOT NULL",
		"auto_apply_approved BOOLEAN NOT NULL DEFAULT FALSE",
		"token_hash VARCHAR(128) NOT NULL UNIQUE",
		"idx_wika_user_tokens_user_tenant_active",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}

func TestWikaP1bKnowledgeMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "versioned", "000092_wika_knowledge_state_access.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Wika P1b knowledge migration to exist: %v", err)
	}

	sql := string(raw)
	required := []string{
		"CREATE TABLE IF NOT EXISTS wika_knowledge_state",
		"knowledge_id VARCHAR(36) PRIMARY KEY",
		"quality_score INT NOT NULL DEFAULT 0 CHECK (quality_score BETWEEN 0 AND 100)",
		"freshness_status VARCHAR(24) NOT NULL DEFAULT 'fresh'",
		"confidence_score NUMERIC(4,3)",
		"ux_wika_knowledge_state_idempotency",
		"CREATE TABLE IF NOT EXISTS knowledge_access_daily",
		"PRIMARY KEY (tenant_id, kb_id, knowledge_id, day)",
		"access_count BIGINT NOT NULL DEFAULT 0 CHECK (access_count >= 0)",
		"idx_knowledge_access_daily_kb_day",
		"idx_knowledge_access_daily_knowledge_day",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}

func TestWikaP1cSuggestionMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "versioned", "000093_wika_suggestions.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Wika P1c suggestion migration to exist: %v", err)
	}

	sql := string(raw)
	required := []string{
		"CREATE TABLE IF NOT EXISTS knowledge_suggestions",
		"CREATE TABLE IF NOT EXISTS knowledge_lineage",
		"source_content_hash VARCHAR(128) NOT NULL",
		"ai_decision VARCHAR(32) NOT NULL",
		"human_decision VARCHAR(32)",
		"human_reviewer_id VARCHAR(64)",
		"human_comment TEXT",
		"status VARCHAR(32) NOT NULL",
		"policy_version INT NOT NULL DEFAULT 1",
		"ux_knowledge_suggestions_open",
		"ux_knowledge_suggestions_idempotency",
		"idx_knowledge_lineage_source",
		"idx_knowledge_lineage_target",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}

func TestWikaP2EvaluationMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "versioned", "000094_wika_evaluation.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Wika P2 evaluation migration to exist: %v", err)
	}

	sql := string(raw)
	required := []string{
		"CREATE TABLE IF NOT EXISTS eval_datasets",
		"CREATE TABLE IF NOT EXISTS eval_qa_items",
		"CREATE TABLE IF NOT EXISTS eval_runs",
		"CREATE TABLE IF NOT EXISTS eval_run_items",
		"expected_knowledge_ids JSONB NOT NULL DEFAULT '[]'::jsonb",
		"expected_chunk_ids JSONB NOT NULL DEFAULT '[]'::jsonb",
		"dataset_version INT NOT NULL DEFAULT 1",
		"idx_eval_qa_items_dataset_enabled",
		"idx_eval_runs_kb_created",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}

func TestWikaP3FreshnessMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "versioned", "000095_wika_freshness.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Wika P3 freshness migration to exist: %v", err)
	}

	sql := string(raw)
	required := []string{
		"CREATE TABLE IF NOT EXISTS freshness_checks",
		"CREATE TABLE IF NOT EXISTS freshness_check_items",
		"issue_type VARCHAR(32) NOT NULL",
		"status VARCHAR(24) NOT NULL DEFAULT 'open'",
		"resolution_action VARCHAR(32)",
		"previous_status VARCHAR(24)",
		"resolved_by VARCHAR(64)",
		"idx_freshness_checks_kb_created",
		"idx_freshness_check_items_knowledge",
		"idx_freshness_check_items_status",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}

func TestWikaP4GraphMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "versioned", "000096_wika_graph.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Wika P4 graph migration to exist: %v", err)
	}

	sql := string(raw)
	required := []string{
		"CREATE TABLE IF NOT EXISTS wika_graph_entities",
		"CREATE TABLE IF NOT EXISTS wika_graph_edges",
		"entity_key VARCHAR(255) NOT NULL",
		"source_knowledge_ids JSONB NOT NULL DEFAULT '[]'::jsonb",
		"evidence_knowledge_id VARCHAR(36)",
		"evidence_text TEXT",
		"ux_wika_graph_entities_key",
		"idx_wika_graph_entities_type_name",
		"idx_wika_graph_edges_source",
		"idx_wika_graph_edges_target",
		"idx_wika_graph_edges_evidence_knowledge",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}

func TestWikaP5ConflictMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "versioned", "000097_wika_conflicts.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Wika P5 conflict migration to exist: %v", err)
	}

	sql := string(raw)
	required := []string{
		"CREATE TABLE IF NOT EXISTS wika_conflict_checks",
		"CREATE TABLE IF NOT EXISTS wika_conflict_items",
		"attempts INT NOT NULL DEFAULT 0",
		"locked_until TIMESTAMPTZ",
		"locked_by VARCHAR(128)",
		"source_knowledge_id VARCHAR(36) NOT NULL",
		"target_knowledge_id VARCHAR(36) NOT NULL",
		"conflict_type VARCHAR(32) NOT NULL",
		"evidence JSONB NOT NULL DEFAULT '{}'::jsonb",
		"status VARCHAR(24) NOT NULL DEFAULT 'open'",
		"reviewer_comment TEXT",
		"ux_wika_conflict_items_open",
		"WHERE status IN ('open', 'confirmed')",
		"idx_wika_conflict_checks_worker",
		"idx_wika_conflict_items_status",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}

func TestWikaP5URLRefreshMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "versioned", "000099_wika_url_refresh.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Wika P5 URL refresh migration to exist: %v", err)
	}

	sql := string(raw)
	required := []string{
		"CREATE TABLE IF NOT EXISTS wika_url_refresh_jobs",
		"CREATE TABLE IF NOT EXISTS wika_url_refresh_schedules",
		"status VARCHAR(24) NOT NULL DEFAULT 'pending'",
		"CHECK (status IN ('pending', 'running', 'pending_review', 'applied', 'rejected', 'failed'))",
		"source_url TEXT NOT NULL",
		"scheduled_for TIMESTAMPTZ",
		"idempotency_key VARCHAR(128)",
		"fetched_hash VARCHAR(128)",
		"diff_summary JSONB NOT NULL DEFAULT '{}'::jsonb",
		"ssrf_check JSONB NOT NULL DEFAULT '{}'::jsonb",
		"attempts INT NOT NULL DEFAULT 0",
		"locked_until TIMESTAMPTZ",
		"locked_by VARCHAR(128)",
		"consecutive_failures INT NOT NULL DEFAULT 0",
		"last_failure_code VARCHAR(64)",
		"ux_wika_url_refresh_jobs_schedule_slot",
		"WHERE schedule_id IS NOT NULL",
		"ux_wika_url_refresh_schedules_enabled",
		"WHERE enabled = TRUE",
		"idx_wika_url_refresh_jobs_worker",
		"idx_wika_url_refresh_schedules_due",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}

func TestWikaP5EvalScheduleMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "versioned", "000100_wika_eval_schedule.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Wika P5 eval schedule migration to exist: %v", err)
	}

	sql := string(raw)
	required := []string{
		"CREATE TABLE IF NOT EXISTS wika_eval_schedules",
		"dataset_id BIGINT NOT NULL REFERENCES eval_datasets(id)",
		"cron_expr VARCHAR(128) NOT NULL",
		"next_run_at TIMESTAMPTZ NOT NULL",
		"consecutive_failures INT NOT NULL DEFAULT 0",
		"last_failure_code VARCHAR(64)",
		"locked_until TIMESTAMPTZ",
		"locked_by VARCHAR(128)",
		"schedule_id BIGINT",
		"scheduled_for TIMESTAMPTZ",
		"ux_wika_eval_schedules_enabled",
		"WHERE enabled = TRUE",
		"ux_eval_runs_schedule_slot",
		"WHERE schedule_id IS NOT NULL",
		"idx_wika_eval_schedules_due",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}
