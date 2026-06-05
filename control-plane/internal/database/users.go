package database

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func CreateUser(username, password string, systemRoleID *uint, email *string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &User{
		Username:     username,
		PasswordHash: string(hash),
		Email:        email,
		SystemRoleID: systemRoleID,
	}
	if err := DB.Create(u).Error; err != nil {
		return nil, err
	}
	return u, nil
}

func GetUserByEmail(email string) (*User, error) {
	var u User
	if err := DB.Preload("SystemRole.Permissions").Where("email = ?", email).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func GetUserByUsername(username string) (*User, error) {
	var u User
	if err := DB.Preload("SystemRole.Permissions").Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func GetUserByID(id uint) (*User, error) {
	var u User
	if err := DB.Preload("SystemRole.Permissions").First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func ListUsers() ([]User, error) {
	var users []User
	if err := DB.Preload("SystemRole").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func UpdateUser(id uint, updates map[string]any) error {
	return DB.Model(&User{}).Where("id = ?", id).Updates(updates).Error
}

func DeleteUser(id uint) error {
	DB.Where("user_id = ?", id).Delete(&HumanGroupMember{})
	DeleteBindingsForPrincipal(PrincipalUser, id)
	return DB.Delete(&User{}, id).Error
}

func CheckPassword(u *User, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func AdminRoleID() (uint, error) {
	var role Role
	if err := DB.Where("name = ? AND scope = ?", "admin", "system").First(&role).Error; err != nil {
		return 0, err
	}
	return role.ID, nil
}

// UserHasSystemPerm returns true if the user's system role contains the given permission.
func UserHasSystemPerm(userID uint, perm string) bool {
	var count int64
	DB.Table("role_permissions").
		Joins("JOIN users ON users.system_role_id = role_permissions.role_id").
		Where("users.id = ? AND role_permissions.permission = ?", userID, perm).
		Count(&count)
	return count > 0
}

// groupIDsForUser returns the IDs of every group the user belongs to.
func groupIDsForUser(userID uint) []uint {
	var ids []uint
	DB.Table("human_group_members").
		Where("user_id = ?", userID).
		Pluck("group_id", &ids)
	return ids
}

// hasBindingPerm reports whether any of the given principals holds a role that
// includes perm on the (resourceType, resourceID) resource.
func hasBindingPerm(principals []bindingPrincipal, resourceType, resourceID, perm string) bool {
	if len(principals) == 0 {
		return false
	}
	q := DB.Table("access_bindings").
		Joins("JOIN role_permissions ON role_permissions.role_id = access_bindings.role_id").
		Where("access_bindings.resource_type = ? AND access_bindings.resource_id = ? AND role_permissions.permission = ?",
			resourceType, resourceID, perm)
	q = q.Where(principalClause(principals))
	var count int64
	q.Count(&count)
	return count > 0
}

type bindingPrincipal struct {
	Type string
	ID   uint
}

// principalsForUser returns the user plus every group the user is a member of,
// expressed as access-binding principals.
func principalsForUser(userID uint) []bindingPrincipal {
	principals := []bindingPrincipal{{Type: PrincipalUser, ID: userID}}
	for _, gid := range groupIDsForUser(userID) {
		principals = append(principals, bindingPrincipal{Type: PrincipalGroup, ID: gid})
	}
	return principals
}

// principalClause builds an OR of (principal_type, principal_id) tuples for a WHERE.
func principalClause(principals []bindingPrincipal) *gorm.DB {
	clause := DB
	for i, p := range principals {
		cond := DB.Where("access_bindings.principal_type = ? AND access_bindings.principal_id = ?", p.Type, p.ID)
		if i == 0 {
			clause = clause.Where(cond)
		} else {
			clause = clause.Or(cond)
		}
	}
	return clause
}

// profileOfAgent returns the agent's profile name, or "" if none/unknown.
func profileOfAgent(agentID string) string {
	var profiles []string
	DB.Model(&Agent{}).Where("id = ? AND agent_profile IS NOT NULL", agentID).
		Pluck("agent_profile", &profiles)
	if len(profiles) == 0 {
		return ""
	}
	return profiles[0]
}

// UserHasProfilePerm returns true if the user (directly or via a group) has the
// given permission on the profile.
func UserHasProfilePerm(userID uint, profileName, perm string) bool {
	// Admin system role bypasses resource ACLs.
	if UserHasSystemPerm(userID, PermManageUsers) {
		return true
	}
	return hasBindingPerm(principalsForUser(userID), ResourceProfile, profileName, perm)
}

// UserHasAgentPerm returns true if the user (directly or via a group) has the
// given permission on a specific agent — either through an agent-level grant or,
// additively, through a grant on the agent's profile.
func UserHasAgentPerm(userID uint, agentID, perm string) bool {
	if UserHasSystemPerm(userID, PermManageUsers) {
		return true
	}
	principals := principalsForUser(userID)
	if hasBindingPerm(principals, ResourceAgent, agentID, perm) {
		return true
	}
	if profile := profileOfAgent(agentID); profile != "" {
		return hasBindingPerm(principals, ResourceProfile, profile, perm)
	}
	return false
}

// VisibleProfiles returns the profile names the user can view, or nil if all (admin).
func VisibleProfiles(userID uint) ([]string, bool) {
	if UserHasSystemPerm(userID, PermManageUsers) {
		return nil, true // admin — sees all
	}
	var profiles []string
	DB.Table("access_bindings").
		Joins("JOIN role_permissions ON role_permissions.role_id = access_bindings.role_id").
		Where("access_bindings.resource_type = ? AND role_permissions.permission = ?", ResourceProfile, PermViewAgents).
		Where(principalClause(principalsForUser(userID))).
		Distinct().
		Pluck("access_bindings.resource_id", &profiles)
	return profiles, false
}

// VisibleAgentIDs returns the specific agent ids the user can view via an
// agent-level grant (not counting profile-inherited visibility).
func VisibleAgentIDs(userID uint) []string {
	if UserHasSystemPerm(userID, PermManageUsers) {
		return nil
	}
	var ids []string
	DB.Table("access_bindings").
		Joins("JOIN role_permissions ON role_permissions.role_id = access_bindings.role_id").
		Where("access_bindings.resource_type = ? AND role_permissions.permission = ?", ResourceAgent, PermViewAgents).
		Where(principalClause(principalsForUser(userID))).
		Distinct().
		Pluck("access_bindings.resource_id", &ids)
	return ids
}

// --- Roles ---

func ListRoles() ([]Role, error) {
	var roles []Role
	if err := DB.Preload("Permissions").Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

func GetRole(id uint) (*Role, error) {
	var role Role
	if err := DB.Preload("Permissions").First(&role, id).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func CreateRole(name, description, scope string) (*Role, error) {
	role := &Role{Name: name, Description: description, Scope: scope}
	if err := DB.Create(role).Error; err != nil {
		return nil, err
	}
	return role, nil
}

func UpdateRole(id uint, updates map[string]any) error {
	return DB.Model(&Role{}).Where("id = ? AND system_default = false", id).Updates(updates).Error
}

func DeleteRole(id uint) error {
	var role Role
	if err := DB.First(&role, id).Error; err != nil {
		return err
	}
	if role.SystemDefault {
		return errors.New("cannot delete a system default role")
	}
	DB.Where("role_id = ?", id).Delete(&RolePermission{})
	return DB.Delete(&Role{}, id).Error
}

func AddRolePermission(roleID uint, perm string) error {
	return DB.FirstOrCreate(&RolePermission{}, RolePermission{RoleID: roleID, Permission: perm}).Error
}

func RemoveRolePermission(roleID uint, perm string) error {
	return DB.Where("role_id = ? AND permission = ?", roleID, perm).Delete(&RolePermission{}).Error
}

// --- Access bindings (resource ACLs) ---

// SetBinding upserts a (principal, resource) → role grant.
func SetBinding(principalType string, principalID uint, resourceType, resourceID string, roleID uint) error {
	b := AccessBinding{
		PrincipalType: principalType, PrincipalID: principalID,
		ResourceType: resourceType, ResourceID: resourceID,
	}
	return DB.Where(b).Assign(AccessBinding{RoleID: roleID}).FirstOrCreate(&AccessBinding{}, b).Error
}

// DeleteBinding removes a (principal, resource) grant.
func DeleteBinding(principalType string, principalID uint, resourceType, resourceID string) error {
	return DB.Where(
		"principal_type = ? AND principal_id = ? AND resource_type = ? AND resource_id = ?",
		principalType, principalID, resourceType, resourceID,
	).Delete(&AccessBinding{}).Error
}

// ListBindings returns all grants on a resource, with the role preloaded.
func ListBindings(resourceType, resourceID string) ([]AccessBinding, error) {
	var bindings []AccessBinding
	if err := DB.Preload("Role").
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Find(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}

// DeleteBindingsForResource removes every grant on a resource (used when the
// resource itself is deleted).
func DeleteBindingsForResource(resourceType, resourceID string) error {
	return DB.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Delete(&AccessBinding{}).Error
}

// ListBindingsForPrincipal returns every grant held directly by a principal
// (a user or a group), with the role preloaded.
func ListBindingsForPrincipal(principalType string, principalID uint) ([]AccessBinding, error) {
	var bindings []AccessBinding
	if err := DB.Preload("Role").
		Where("principal_type = ? AND principal_id = ?", principalType, principalID).
		Find(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}

// GroupsForUser returns every group the user belongs to.
func GroupsForUser(userID uint) ([]HumanGroup, error) {
	var groups []HumanGroup
	err := DB.Joins("JOIN human_group_members ON human_group_members.group_id = human_groups.id").
		Where("human_group_members.user_id = ?", userID).
		Find(&groups).Error
	return groups, err
}

// migrateProfileACLToBindings copies legacy AgentProfileACL rows into
// AccessBinding once. The source table is left intact for rollback.
func migrateProfileACLToBindings() {
	var acls []AgentProfileACL
	if err := DB.Find(&acls).Error; err != nil {
		return
	}
	for _, a := range acls {
		_ = SetBinding(PrincipalUser, a.UserID, ResourceProfile, a.ProfileName, a.RoleID)
	}
}

// --- Human groups (teams) ---

func ListGroups() ([]HumanGroup, error) {
	var groups []HumanGroup
	return groups, DB.Order("name").Find(&groups).Error
}

func GetGroup(id uint) (*HumanGroup, error) {
	var g HumanGroup
	return &g, DB.First(&g, id).Error
}

func CreateGroup(name, description string) (*HumanGroup, error) {
	g := &HumanGroup{Name: name, Description: description}
	if err := DB.Create(g).Error; err != nil {
		return nil, err
	}
	return g, nil
}

func UpdateGroup(id uint, updates map[string]any) error {
	return DB.Model(&HumanGroup{}).Where("id = ?", id).Updates(updates).Error
}

func DeleteGroup(id uint) error {
	DB.Where("group_id = ?", id).Delete(&HumanGroupMember{})
	DeleteBindingsForPrincipal(PrincipalGroup, id)
	return DB.Delete(&HumanGroup{}, id).Error
}

// DeleteBindingsForPrincipal removes every grant held by a principal (used when
// a group or user is deleted).
func DeleteBindingsForPrincipal(principalType string, principalID uint) error {
	return DB.Where("principal_type = ? AND principal_id = ?", principalType, principalID).
		Delete(&AccessBinding{}).Error
}

func ListGroupMembers(groupID uint) ([]User, error) {
	var users []User
	err := DB.Preload("SystemRole").
		Joins("JOIN human_group_members ON human_group_members.user_id = users.id").
		Where("human_group_members.group_id = ?", groupID).
		Find(&users).Error
	return users, err
}

func AddGroupMember(groupID, userID uint) error {
	m := HumanGroupMember{GroupID: groupID, UserID: userID}
	return DB.Where(m).FirstOrCreate(&HumanGroupMember{}, m).Error
}

func RemoveGroupMember(groupID, userID uint) error {
	return DB.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&HumanGroupMember{}).Error
}

// UserCount returns total number of users.
func UserCount() (int64, error) {
	var count int64
	return count, DB.Model(&User{}).Count(&count).Error
}

// LinkUserIdentity associates an external provider identity with a user.
func LinkUserIdentity(userID uint, provider, externalID string) error {
	identity := UserIdentity{UserID: userID, Provider: provider, ExternalID: externalID}
	return DB.Where(UserIdentity{UserID: userID, Provider: provider}).
		Assign(UserIdentity{ExternalID: externalID}).
		FirstOrCreate(&identity).Error
}

// ListUserIdentities returns all external identities linked to a user.
func ListUserIdentities(userID uint) ([]UserIdentity, error) {
	var identities []UserIdentity
	if err := DB.Where("user_id = ?", userID).Find(&identities).Error; err != nil {
		return nil, err
	}
	return identities, nil
}

// UnlinkUserIdentity removes a specific provider identity from a user.
func UnlinkUserIdentity(userID uint, provider string) error {
	return DB.Where("user_id = ? AND provider = ?", userID, provider).Delete(&UserIdentity{}).Error
}

// GetUserByExternalIdentity resolves an external provider identity to a user.
func GetUserByExternalIdentity(provider, externalID string) (*User, error) {
	var identity UserIdentity
	if err := DB.Where("provider = ? AND external_id = ?", provider, externalID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return GetUserByID(identity.UserID)
}
