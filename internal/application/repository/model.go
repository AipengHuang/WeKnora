package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// modelRepository implements the model repository interface
type modelRepository struct {
	db *gorm.DB
}

// NewModelRepository creates a new model repository
func NewModelRepository(db *gorm.DB) interfaces.ModelRepository {
	return &modelRepository{db: db}
}

// Create creates a new model
func (r *modelRepository) Create(ctx context.Context, m *types.Model) error {
	if err := validatePersistedModelProtocol(m); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByID retrieves a model by ID
func (r *modelRepository) GetByID(ctx context.Context, tenantID uint64, id string) (*types.Model, error) {
	var m types.Model
	if err := r.db.WithContext(ctx).Where("id = ?", id).Where(
		"(tenant_id = ? OR is_builtin = true)", tenantID,
	).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// List lists models with optional filtering
func (r *modelRepository) List(
	ctx context.Context, tenantID uint64, modelType types.ModelType, source types.ModelSource,
) ([]*types.Model, error) {
	var models []*types.Model
	query := r.db.WithContext(ctx).Where(
		"(tenant_id = ? OR is_builtin = true)", tenantID,
	)

	if modelType != "" {
		query = query.Where("type = ?", modelType)
	}

	if source != "" {
		query = query.Where("source = ?", source)
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	return models, nil
}

// Update updates a model
func (r *modelRepository) Update(ctx context.Context, m *types.Model) error {
	if err := validatePersistedModelProtocol(m); err != nil {
		return err
	}
	// Use Select to explicitly update all fields, including zero values like false
	return r.db.WithContext(ctx).Debug().Model(&types.Model{}).Where(
		"id = ? AND tenant_id = ?", m.ID, m.TenantID,
	).Select("*").Updates(m).Error
}

// Delete deletes a model
func (r *modelRepository) Delete(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).Where(
		"id = ? AND tenant_id = ?", id, tenantID,
	).Delete(&types.Model{}).Error
}

// ClearDefaultByType clears the default flag for all models of a specific type
// This is a batch operation that updates all matching records in one query
func (r *modelRepository) ClearDefaultByType(
	ctx context.Context,
	tenantID uint64,
	modelType types.ModelType,
	excludeID string,
) error {
	return clearDefaultByType(r.db.WithContext(ctx), tenantID, modelType, excludeID)
}

// SetDefault 将同一租户和类型的模型更新串行化，避免并发产生多个默认模型。
func (r *modelRepository) SetDefault(
	ctx context.Context,
	tenantID uint64,
	modelID string,
	modelType types.ModelType,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 无变化更新会锁定该类型的全部现有模型，并同时兼容 PostgreSQL 与 SQLite。
		if err := tx.Model(&types.Model{}).
			Where("tenant_id = ? AND type = ?", tenantID, modelType).
			UpdateColumn("is_default", gorm.Expr("is_default")).Error; err != nil {
			return err
		}
		if err := clearDefaultByType(tx, tenantID, modelType, ""); err != nil {
			return err
		}

		result := tx.Model(&types.Model{}).
			Where("id = ? AND tenant_id = ? AND type = ?", modelID, tenantID, modelType).
			Update("is_default", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func clearDefaultByType(
	db *gorm.DB,
	tenantID uint64,
	modelType types.ModelType,
	excludeID string,
) error {
	query := db.Model(&types.Model{}).Where(
		"tenant_id = ? AND type = ? AND is_default = ?", tenantID, modelType, true,
	)

	// 仅在调用方明确传入目标模型时保留该模型。
	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}

	return query.Update("is_default", false).Error
}
