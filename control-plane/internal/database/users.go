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

// UserHasProfilePerm returns true if the user has the given permission on the profile (or on "*").
func UserHasProfilePerm(userID uint, profileName, perm string) bool {
	// Admin system role bypasses profile ACL
	if UserHasSystemPerm(userID, PermManageUsers) {
		return true
	}
	var count int64
	DB.Table("role_permissions").
		Joins("JOIN agent_profile_acls ON agent_profile_acls.role_id = role_permissions.role_id").
		Where("agent_profile_acls.user_id = ? AND agent_profile_acls.profile_name = ? AND role_permissions.permission = ?",
			userID, profileName, perm).
		Count(&count)
	return count > 0
}

// VisibleProfiles returns the profile names the user can view, or nil if all (admin).
func VisibleProfiles(userID uint) ([]string, bool) {
	if UserHasSystemPerm(userID, PermManageUsers) {
		return nil, true // admin — sees all
	}
	var profiles []string
	DB.Table("agent_profile_acls").
		Joins("JOIN role_permissions ON role_permissions.role_id = agent_profile_acls.role_id").
		Where("agent_profile_acls.user_id = ? AND role_permissions.permission = ?", userID, PermViewAgents).
		Pluck("agent_profile_acls.profile_name", &profiles)
	return profiles, false
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

// --- Profile ACL ---

func SetProfileACL(profileName string, userID, roleID uint) error {
	acl := AgentProfileACL{ProfileName: profileName, UserID: userID, RoleID: roleID}
	return DB.Save(&acl).Error
}

func DeleteProfileACL(profileName string, userID uint) error {
	return DB.Where("profile_name = ? AND user_id = ?", profileName, userID).Delete(&AgentProfileACL{}).Error
}

func ListProfileACL(profileName string) ([]AgentProfileACL, error) {
	var acls []AgentProfileACL
	if err := DB.Preload("Role").Where("profile_name = ?", profileName).Find(&acls).Error; err != nil {
		return nil, err
	}
	return acls, nil
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
