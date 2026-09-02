package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// PutExternalTenant 使用一个持久化外部标识完成远端幂等租户创建。
func (s *tenantService) PutExternalTenant(
	ctx context.Context,
	ref types.ExternalTenantRef,
	tenant *types.Tenant,
) (*types.Tenant, bool, error) {
	if tenant == nil || tenant.Name == "" {
		return nil, false, errors.New("workspace name cannot be empty")
	}
	tenant.ID = 0
	tenant.Status = "active"
	tenant.CreatedAt = time.Now()
	tenant.UpdatedAt = tenant.CreatedAt
	backend, err := types.ExternalTenantStorageBackendFromEnvironment(tenant.ID)
	if err != nil {
		return nil, false, fmt.Errorf("invalid external tenant storage backend: %w", err)
	}
	return s.repo.PutExternalTenant(ctx, ref, tenant, backend)
}
