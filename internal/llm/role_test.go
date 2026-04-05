package llm

import "testing"

func TestAllRolesContainsExpected(t *testing.T) {
	roles := AllRoles()
	want := []Role{
		RoleChatMain,
		RoleContextCompactor,
		RoleMemoryHook,
		RoleReflectionMemory,
		RoleReflectionKB,
		RolePulseDecider,
		RoleGatewayDefault,
		RoleGatewayPlanner,
	}
	if len(roles) != len(want) {
		t.Fatalf("AllRoles length = %d, want %d", len(roles), len(want))
	}
	for i, role := range want {
		if roles[i] != role {
			t.Errorf("AllRoles[%d] = %q, want %q", i, roles[i], role)
		}
	}
}

func TestRoleValid(t *testing.T) {
	for _, role := range AllRoles() {
		if !role.Valid() {
			t.Errorf("AllRoles() returned invalid role %q", role)
		}
	}
	if Role("unknown_role").Valid() {
		t.Error("Role(unknown_role).Valid() should be false")
	}
}

func TestParseRole(t *testing.T) {
	if _, ok := ParseRole("chat_main"); !ok {
		t.Error("ParseRole(chat_main) failed")
	}
	if _, ok := ParseRole("  Pulse_Decider  "); !ok {
		t.Error("ParseRole should be case-insensitive and trimmed")
	}
	if _, ok := ParseRole(""); ok {
		t.Error("ParseRole(empty) should fail")
	}
	if _, ok := ParseRole("not_a_role"); ok {
		t.Error("ParseRole(not_a_role) should fail")
	}
}
