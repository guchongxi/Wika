package types

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestModelScopeConstantsAndFields(t *testing.T) {
	if ModelScopeSystem != "system" {
		t.Fatalf("ModelScopeSystem = %q, want system", ModelScopeSystem)
	}
	if ModelScopeTenant != "tenant" {
		t.Fatalf("ModelScopeTenant = %q, want tenant", ModelScopeTenant)
	}
	if ModelScopeUser != "user" {
		t.Fatalf("ModelScopeUser = %q, want user", ModelScopeUser)
	}

	modelType := reflect.TypeOf(Model{})
	for _, field := range []string{"Scope", "OwnerUserID", "UserSelectable"} {
		if _, ok := modelType.FieldByName(field); !ok {
			t.Fatalf("types.Model missing %s field", field)
		}
	}
}

func TestModelScopeMigrationDeclaresOwnershipAndVisibility(t *testing.T) {
	body, err := os.ReadFile("../../migrations/versioned/000106_model_scope_visibility.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(body)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS scope",
		"ADD COLUMN IF NOT EXISTS owner_user_id",
		"ADD COLUMN IF NOT EXISTS user_selectable",
		"chk_models_scope",
		"idx_models_system_default_type",
		"WHERE is_builtin = TRUE",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
}
