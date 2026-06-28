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
		"ai_decision VARCHAR(32) NOT NULL",
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
