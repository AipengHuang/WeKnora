package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type externalTenantRepositoryStub struct {
	interfaces.TenantRepository
	tenant  *types.Tenant
	backend *types.StorageBackend
}

func (s *externalTenantRepositoryStub) PutExternalTenant(
	_ context.Context,
	ref types.ExternalTenantRef,
	tenant *types.Tenant,
	backend *types.StorageBackend,
) (*types.Tenant, bool, error) {
	externalRef := ref.String()
	tenant.ExternalRef = &externalRef
	s.tenant = tenant
	s.backend = backend
	tenant.ID = 10001
	return tenant, true, nil
}

func TestExternalTenantServiceUsesOneCanonicalReference(t *testing.T) {
	t.Setenv("STORAGE_TYPE", "local")
	repo := &externalTenantRepositoryStub{}
	service := NewTenantService(repo, nil)
	ref, err := types.ParseExternalTenantRef("7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7")
	require.NoError(t, err)
	stored, created, err := service.PutExternalTenant(
		context.Background(), ref, &types.Tenant{Name: "Workspace", Description: "Knowledge"},
	)
	require.NoError(t, err)
	require.True(t, created)
	require.EqualValues(t, 10001, stored.ID)
	require.Equal(t, ref.String(), *repo.tenant.ExternalRef)
	require.Equal(t, "active", repo.tenant.Status)
	require.Equal(t, "local", repo.backend.Provider)
}
