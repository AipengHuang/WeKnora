package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type ExternalTenantRepository interface {
	PutExternalTenant(
		ctx context.Context,
		ref types.ExternalTenantRef,
		tenant *types.Tenant,
		backend *types.StorageBackend,
	) (*types.Tenant, bool, error)
}

type ExternalTenantService interface {
	PutExternalTenant(
		ctx context.Context,
		ref types.ExternalTenantRef,
		tenant *types.Tenant,
	) (*types.Tenant, bool, error)
}
