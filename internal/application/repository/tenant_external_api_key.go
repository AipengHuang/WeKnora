package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PutExternalTenantAPIKey 由外部引用唯一约束裁决并发，并返回同一已加密凭据。
func (r *tenantAPIKeyRepository) PutExternalTenantAPIKey(
	ctx context.Context,
	tenantRef types.ExternalTenantRef,
	credentialRef types.ExternalTenantCredentialRef,
	key *types.TenantAPIKey,
) (*types.TenantAPIKey, bool, error) {
	var tenant types.Tenant
	err := r.db.WithContext(ctx).Unscoped().
		Select("id", "deleted_at").
		Where("external_ref = ?", tenantRef.String()).
		First(&tenant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, types.ErrExternalTenantNotFound
	}
	if err != nil {
		return nil, false, err
	}
	if tenant.DeletedAt.Valid {
		return nil, false, types.ErrExternalTenantDeleted
	}

	token := key.APIKey
	var stored *types.TenantAPIKey
	created := false
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		externalRef := credentialRef.String()
		key.TenantID = &tenant.ID
		key.ExternalRef = &externalRef
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "external_ref"}},
			DoNothing: true,
		}).Create(key)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			key.APIKey = token
			stored = key
			created = true
			return nil
		}

		var existing types.TenantAPIKey
		if err := tx.Where("external_ref = ?", externalRef).First(&existing).Error; err != nil {
			return err
		}
		if existing.TenantID == nil || *existing.TenantID != tenant.ID {
			return types.ErrExternalTenantCredentialConflict
		}
		stored = &existing
		return nil
	})
	return stored, created, err
}
