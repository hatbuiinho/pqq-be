package auth

const (
	SystemRoleUser     = "user"
	SystemRoleSysAdmin = "sys_admin"

	ClubRoleOwner     = "owner"
	ClubRoleAssistant = "assistant"

	PermissionClubRead         = "club:read"
	PermissionClubManage       = "club:manage"
	PermissionMembershipManage = "membership:manage"
	PermissionStudentsRead     = "students:read"
	PermissionStudentsWrite    = "students:write"
	PermissionStudentsDelete   = "students:delete"
	PermissionAttendanceRead   = "attendance:read"
	PermissionAttendanceWrite  = "attendance:write"
	PermissionAttendanceReopen = "attendance:reopen"
	PermissionBeltRanksRead    = "belt_ranks:read"
	PermissionBeltRanksWrite   = "belt_ranks:write"
	PermissionClubGroupsRead   = "club_groups:read"
	PermissionClubGroupsWrite  = "club_groups:write"
	PermissionImportsManage    = "imports:manage"
	PermissionMediaManage      = "media:manage"
	PermissionAuditLogsRead    = "audit_logs:read"
	PermissionUsersManage      = "users:manage"
)

var allPermissions = []string{
	PermissionClubRead,
	PermissionClubManage,
	PermissionMembershipManage,
	PermissionStudentsRead,
	PermissionStudentsWrite,
	PermissionStudentsDelete,
	PermissionAttendanceRead,
	PermissionAttendanceWrite,
	PermissionAttendanceReopen,
	PermissionBeltRanksRead,
	PermissionBeltRanksWrite,
	PermissionClubGroupsRead,
	PermissionClubGroupsWrite,
	PermissionImportsManage,
	PermissionMediaManage,
	PermissionAuditLogsRead,
	PermissionUsersManage,
}

func EvaluatePermissions(systemRole string, clubRole string) map[string]bool {
	permissions := make(map[string]bool, len(allPermissions))
	for _, permission := range allPermissions {
		permissions[permission] = false
	}

	if systemRole == SystemRoleSysAdmin {
		for _, permission := range allPermissions {
			permissions[permission] = true
		}
		return permissions
	}

	switch clubRole {
	case ClubRoleOwner:
		for _, permission := range allPermissions {
			permissions[permission] = true
		}
	case ClubRoleAssistant:
		permissions[PermissionClubRead] = true
		permissions[PermissionStudentsRead] = true
		permissions[PermissionStudentsWrite] = true
		permissions[PermissionAttendanceRead] = true
		permissions[PermissionAttendanceWrite] = true
		permissions[PermissionBeltRanksRead] = true
		permissions[PermissionBeltRanksWrite] = true
		permissions[PermissionClubGroupsRead] = true
		permissions[PermissionClubGroupsWrite] = true
		permissions[PermissionImportsManage] = true
		permissions[PermissionMediaManage] = true
	}

	return permissions
}

func HasPermission(systemRole string, clubRole string, permission string) bool {
	return EvaluatePermissions(systemRole, clubRole)[permission]
}
