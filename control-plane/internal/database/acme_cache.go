package database

import (
	"context"

	"golang.org/x/crypto/acme/autocert"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DBCertCache implements autocert.Cache backed by the database.
type DBCertCache struct{}

func (DBCertCache) Get(ctx context.Context, key string) ([]byte, error) {
	var row AcmeCache
	err := DB.WithContext(ctx).First(&row, "key = ?", key).Error
	if err == gorm.ErrRecordNotFound {
		return nil, autocert.ErrCacheMiss
	}
	if err != nil {
		return nil, err
	}
	return row.Data, nil
}

func (DBCertCache) Put(ctx context.Context, key string, data []byte) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"data", "updated_at"}),
	}).Create(&AcmeCache{Key: key, Data: data}).Error
}

func (DBCertCache) Delete(ctx context.Context, key string) error {
	return DB.WithContext(ctx).Delete(&AcmeCache{}, "key = ?", key).Error
}
