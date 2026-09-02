package repository

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PutExternalTenant 由数据库唯一约束裁决并发，并在一个事务内提交完整租户。
func (r *tenantRepository) PutExternalTenant(
	ctx context.Context,
	ref types.ExternalTenantRef,
	tenant *types.Tenant,
	backend *types.StorageBackend,
) (*types.Tenant, bool, error) {
	externalRef := ref.String()
	tenant.ExternalRef = &externalRef
	var stored *types.Tenant
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "external_ref"}},
			DoNothing: true,
		}).Create(tenant)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var existing types.Tenant
			if err := tx.Unscoped().Where("external_ref = ?", externalRef).First(&existing).Error; err != nil {
				return err
			}
			if existing.DeletedAt.Valid {
				return types.ErrExternalTenantDeleted
			}
			stored = &existing
			return nil
		}

		backend.TenantID = tenant.ID
		backend.LegacyAlias = true
		if err := backend.Validate(); err != nil {
			return err
		}
		if err := tx.Create(backend).Error; err != nil {
			return err
		}
		tenant.DefaultStorageBackendID = &backend.ID
		if err := tx.Model(&types.Tenant{}).
			Where("id = ?", tenant.ID).
			Update("default_storage_backend_id", backend.ID).Error; err != nil {
			return err
		}
		stored = tenant
		created = true
		return nil
	})
	return stored, created, err
}
