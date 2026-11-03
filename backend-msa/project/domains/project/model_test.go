package project

import "testing"

func TestRolePermissions(t *testing.T) {
	tests := []struct {
		role   Role
		read   bool
		edit   bool
		manage bool
	}{
		{RoleOwner, true, true, true},
		{RoleEditor, true, true, false},
		{RoleViewer, true, false, false},
		{Role("unknown"), false, false, false},
	}
	for _, test := range tests {
		if test.role.CanRead() != test.read || test.role.CanEdit() != test.edit || test.role.CanManageMembers() != test.manage {
			t.Fatalf("permissions for %q = read:%t edit:%t manage:%t", test.role, test.role.CanRead(), test.role.CanEdit(), test.role.CanManageMembers())
		}
	}
}
