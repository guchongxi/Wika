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
