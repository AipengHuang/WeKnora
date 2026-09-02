package repository

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSetDefaultKeepsOneModelPerTenantAndType(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Model{}))
	repo := NewModelRepository(db)
	ctx := context.Background()

	models := []*types.Model{
		{ID: "chat-a", TenantID: 7, Name: "chat-a", Type: types.ModelTypeKnowledgeQA,
			Source: types.ModelSourceLocal, Status: types.ModelStatusActive, IsDefault: true},
		{ID: "chat-b", TenantID: 7, Name: "chat-b", Type: types.ModelTypeKnowledgeQA,
			Source: types.ModelSourceLocal, Status: types.ModelStatusActive},
		{ID: "embed-a", TenantID: 7, Name: "embed-a", Type: types.ModelTypeEmbedding,
			Source: types.ModelSourceLocal, Status: types.ModelStatusActive, IsDefault: true},
		{ID: "other-tenant", TenantID: 8, Name: "other-tenant", Type: types.ModelTypeKnowledgeQA,
			Source: types.ModelSourceLocal, Status: types.ModelStatusActive, IsDefault: true},
	}
	require.NoError(t, db.Create(models).Error)
	require.NoError(t, repo.SetDefault(ctx, 7, "chat-b", types.ModelTypeKnowledgeQA))

	var defaults []types.Model
	require.NoError(t, db.Where("is_default = ?", true).Order("id").Find(&defaults).Error)
	require.Len(t, defaults, 3)
	require.Equal(t, "chat-b", defaults[0].ID)
	require.Equal(t, "embed-a", defaults[1].ID)
	require.Equal(t, "other-tenant", defaults[2].ID)
}

func TestSetDefaultRollsBackWhenTargetDoesNotMatchBucket(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Model{}))
	repo := NewModelRepository(db)
	ctx := context.Background()

	model := &types.Model{ID: "chat-a", TenantID: 7, Name: "chat-a", Type: types.ModelTypeKnowledgeQA,
		Source: types.ModelSourceLocal, Status: types.ModelStatusActive, IsDefault: true}
	require.NoError(t, db.Create(model).Error)
	err = repo.SetDefault(ctx, 7, "missing", types.ModelTypeKnowledgeQA)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	var stored types.Model
	require.NoError(t, db.First(&stored, "id = ?", model.ID).Error)
	require.True(t, stored.IsDefault)
}
