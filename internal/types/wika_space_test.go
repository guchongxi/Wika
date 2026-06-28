package types

import "testing"

func TestTenantEnsureSpaceTypeDefaultsEmptyToTeam(t *testing.T) {
	tenant := &Tenant{}

	tenant.EnsureSpaceType()

	if tenant.SpaceType != SpaceTypeTeam {
		t.Fatalf("expected empty tenant space type to default to %q, got %q", SpaceTypeTeam, tenant.SpaceType)
	}
}

func TestSpaceTypeValidation(t *testing.T) {
	if !SpaceTypePersonal.IsValid() {
		t.Fatalf("expected %q to be valid", SpaceTypePersonal)
	}
	if !SpaceTypeTeam.IsValid() {
		t.Fatalf("expected %q to be valid", SpaceTypeTeam)
	}
	if SpaceType("shared").IsValid() {
		t.Fatal("expected unknown space type to be invalid")
	}
}

func TestUserPersonalSpaceTableName(t *testing.T) {
	if got := (UserPersonalSpace{}).TableName(); got != "user_personal_spaces" {
		t.Fatalf("expected user personal space table name, got %q", got)
	}
}

func TestWikaP1aGovernanceTableNames(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "space defaults", got: (WikaSpaceDefault{}).TableName(), want: "wika_space_defaults"},
		{name: "space policies", got: (WikaSpacePolicy{}).TableName(), want: "wika_space_policies"},
		{name: "user tokens", got: (WikaUserToken{}).TableName(), want: "wika_user_tokens"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("expected table name %q, got %q", tc.want, tc.got)
			}
		})
	}
}
