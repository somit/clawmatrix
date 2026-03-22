package database

// System-scoped permissions (global, not tied to a profile)
const (
	PermManageUsers         = "can_manage_users"
	PermManageRoles         = "can_manage_roles"
	PermManageRegistrations = "can_manage_registrations"
	PermManageProfiles      = "can_manage_profiles"
	PermManageConnections   = "can_manage_connections"
	PermManageCrons         = "can_manage_crons"
	PermViewLogs            = "can_view_logs"
	PermViewAudit           = "can_view_audit"
	PermViewMetrics         = "can_view_metrics"
)

// Profile-scoped permissions (checked against agent_profile_acl)
const (
	PermViewAgents      = "can_view_agents"
	PermChatWithAgents  = "can_chat_with_agents"
	PermConfigureAgents = "can_configure_agents"
	PermViewCrons       = "can_view_crons"
	PermTriggerCrons    = "can_trigger_crons"
	PermAddCrons        = "can_add_crons"
	PermEditCrons       = "can_edit_crons"
	PermDeleteCrons     = "can_delete_crons"
)

var allSystemPerms = []string{
	PermManageUsers, PermManageRoles, PermManageRegistrations,
	PermManageProfiles, PermManageConnections, PermManageCrons,
	PermViewLogs, PermViewAudit, PermViewMetrics,
}

var allProfilePerms = []string{
	PermViewAgents, PermChatWithAgents, PermConfigureAgents,
	PermViewCrons, PermTriggerCrons, PermAddCrons, PermEditCrons, PermDeleteCrons,
}

// defaultRoles are seeded on startup and cannot be deleted.
var defaultRoles = []struct {
	Name        string
	Description string
	Scope       string
	Perms       []string
}{
	{
		Name:        "admin",
		Description: "Full system access",
		Scope:       "system",
		Perms:       allSystemPerms,
	},
	{
		Name:        "owner",
		Description: "Full access to an agent profile",
		Scope:       "profile",
		Perms:       allProfilePerms,
	},
	{
		Name:        "operator",
		Description: "Manage and interact with agents",
		Scope:       "profile",
		Perms: []string{
			PermViewAgents, PermChatWithAgents, PermConfigureAgents,
			PermViewCrons, PermTriggerCrons, PermAddCrons, PermEditCrons, PermDeleteCrons,
		},
	},
	{
		Name:        "chatter",
		Description: "Chat with agents and trigger crons",
		Scope:       "profile",
		Perms:       []string{PermViewAgents, PermChatWithAgents, PermViewCrons, PermTriggerCrons},
	},
	{
		Name:        "viewer",
		Description: "Read-only access to agents and crons",
		Scope:       "profile",
		Perms:       []string{PermViewAgents, PermViewCrons},
	},
}

// SeedRoles inserts default roles if they don't exist.
func SeedRoles() {
	for _, r := range defaultRoles {
		var existing Role
		if DB.Where("name = ?", r.Name).First(&existing).Error == nil {
			continue
		}
		role := Role{
			Name:          r.Name,
			Description:   r.Description,
			Scope:         r.Scope,
			SystemDefault: true,
		}
		DB.Create(&role)
		for _, p := range r.Perms {
			DB.Create(&RolePermission{RoleID: role.ID, Permission: p})
		}
	}
}

// RoleHasPerm returns true if the given role ID has the permission.
func RoleHasPerm(roleID uint, perm string) bool {
	var count int64
	DB.Model(&RolePermission{}).
		Where("role_id = ? AND permission = ?", roleID, perm).
		Count(&count)
	return count > 0
}
