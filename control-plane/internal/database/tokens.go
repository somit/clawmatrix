package database

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
)

// TokenPrefix is the human-recognizable prefix for personal access tokens.
const TokenPrefix = "wm_pat_"

// CreateUserToken mints a new PAT for a user. The raw token is returned exactly
// once (only its hash is persisted).
func CreateUserToken(userID uint, name string, expiresAt *time.Time) (string, *UserToken, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	raw := TokenPrefix + hex.EncodeToString(b)
	t := &UserToken{
		UserID:    userID,
		Name:      name,
		TokenHash: HashToken(raw),
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	if err := DB.Create(t).Error; err != nil {
		return "", nil, err
	}
	return raw, t, nil
}

// GetUserByToken resolves a PAT to its owning user, rejecting expired tokens and
// best-effort updating LastUsedAt. SystemRole+permissions are preloaded so the
// returned user works with the permission checks.
func GetUserByToken(token string) (*User, error) {
	if token == "" {
		return nil, errors.New("empty token")
	}
	var t UserToken
	if err := DB.Where("token_hash = ?", HashToken(token)).First(&t).Error; err != nil {
		return nil, err
	}
	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return nil, errors.New("token expired")
	}
	now := time.Now().UTC()
	DB.Model(&UserToken{}).Where("id = ?", t.ID).Update("last_used_at", now)

	var u User
	if err := DB.Preload("SystemRole.Permissions").First(&u, t.UserID).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func ListUserTokens(userID uint) ([]UserToken, error) {
	var tokens []UserToken
	return tokens, DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&tokens).Error
}

// DeleteUserToken removes a token, scoped to its owner so users can only revoke
// their own.
func DeleteUserToken(userID, tokenID uint) error {
	res := DB.Where("id = ? AND user_id = ?", tokenID, userID).Delete(&UserToken{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ChattableProfiles returns the profile names the user may chat with, or nil+true
// if the user is an admin (all profiles).
func ChattableProfiles(userID uint) ([]string, bool) {
	if UserHasSystemPerm(userID, PermManageUsers) {
		return nil, true
	}
	var profiles []string
	DB.Table("agent_profile_acls").
		Joins("JOIN role_permissions ON role_permissions.role_id = agent_profile_acls.role_id").
		Where("agent_profile_acls.user_id = ? AND role_permissions.permission = ?", userID, PermChatWithAgents).
		Pluck("agent_profile_acls.profile_name", &profiles)
	return profiles, false
}
