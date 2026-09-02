package service

import (
	"context"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// SetDefaultModel 将活动模型设为当前租户内同类型的唯一默认模型。
func (s *modelService) SetDefaultModel(ctx context.Context, id string) (*types.Model, error) {
	tenantID := types.MustTenantIDFromContext(ctx)
	model, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if model == nil {
		return nil, ErrModelNotFound
	}
	if model.TenantID != tenantID {
		return nil, apperrors.NewForbiddenError("model does not belong to this workspace")
	}
	if model.Status != types.ModelStatusActive {
		return nil, apperrors.NewBadRequestError("only active models can be set as default")
	}
	if model.IsDefault {
		return model, nil
	}

	if err := s.repo.SetDefault(ctx, tenantID, model.ID, model.Type); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":  model.ID,
			"tenant_id": tenantID,
		})
		return nil, err
	}

	model, err = s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if model == nil {
		return nil, ErrModelNotFound
	}
	return model, nil
}
