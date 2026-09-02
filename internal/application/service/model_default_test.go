package service

import (
	"context"
	"testing"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type defaultModelRepoStub struct {
	interfaces.ModelRepository
	model    *types.Model
	setCalls int
}

func (s *defaultModelRepoStub) GetByID(context.Context, uint64, string) (*types.Model, error) {
	return s.model, nil
}

func (s *defaultModelRepoStub) SetDefault(context.Context, uint64, string, types.ModelType) error {
	s.setCalls++
	s.model.IsDefault = true
	return nil
}

func TestSetDefaultModelPromotesActiveTenantModel(t *testing.T) {
	repo := &defaultModelRepoStub{model: &types.Model{
		ID: "chat-a", TenantID: 7, Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive,
	}}
	svc := NewModelService(repo, nil, nil, nil, nil, nil)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	model, err := svc.SetDefaultModel(ctx, "chat-a")
	require.NoError(t, err)
	require.True(t, model.IsDefault)
	require.Equal(t, 1, repo.setCalls)
}

func TestSetDefaultModelRejectsInactiveAndCrossTenantModels(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	tests := []struct {
		name  string
		model *types.Model
		code  apperrors.ErrorCode
	}{
		{
			name: "inactive",
			model: &types.Model{ID: "chat-a", TenantID: 7, Type: types.ModelTypeKnowledgeQA,
				Status: types.ModelStatusDownloading},
			code: apperrors.ErrBadRequest,
		},
		{
			name: "cross tenant",
			model: &types.Model{ID: "chat-a", TenantID: 8, Type: types.ModelTypeKnowledgeQA,
				Status: types.ModelStatusActive},
			code: apperrors.ErrForbidden,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &defaultModelRepoStub{model: test.model}
			svc := NewModelService(repo, nil, nil, nil, nil, nil)
			_, err := svc.SetDefaultModel(ctx, test.model.ID)
			appErr, ok := apperrors.IsAppError(err)
			require.True(t, ok)
			require.Equal(t, test.code, appErr.Code)
			require.Zero(t, repo.setCalls)
		})
	}
}
