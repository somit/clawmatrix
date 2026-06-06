package database

import "time"

// CreateUpload inserts upload metadata. The blob itself is written to the
// storage backend by the caller; this row records ownership and lets a GC job
// clean up expired files later.
func CreateUpload(u *Upload) error {
	return DB.Create(u).Error
}

// GetUpload returns upload metadata by id.
func GetUpload(id string) (*Upload, error) {
	var u Upload
	if err := DB.First(&u, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ListUploads returns a user's uploads, newest first.
func ListUploads(userID uint) ([]Upload, error) {
	var ups []Upload
	err := DB.Where("user_id = ?", userID).Order("created_at desc").Find(&ups).Error
	return ups, err
}

// DeleteUpload removes an upload row scoped to its owner.
func DeleteUpload(userID uint, id string) error {
	return DB.Where("user_id = ? AND id = ?", userID, id).Delete(&Upload{}).Error
}

// ExpiredUploads returns uploads whose ExpiresAt is in the past (for GC).
func ExpiredUploads(now time.Time) ([]Upload, error) {
	var ups []Upload
	err := DB.Where("expires_at IS NOT NULL AND expires_at < ?", now).Find(&ups).Error
	return ups, err
}
