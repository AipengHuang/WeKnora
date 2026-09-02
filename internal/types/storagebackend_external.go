package types

import (
	"fmt"
	"os"

	"github.com/Tencent/WeKnora/internal/storageallowlist"
)

// ExternalTenantStorageBackendFromEnvironment 严格读取平台租户唯一的存储配置。
func ExternalTenantStorageBackendFromEnvironment(tenantID uint64) (*StorageBackend, error) {
	provider := os.Getenv("STORAGE_TYPE")
	if err := validateExternalTenantStorageProvider(provider); err != nil {
		return nil, err
	}
	backend := StorageBackendFromEnvironment(tenantID)
	if backend == nil || backend.Provider != provider {
		return nil, fmt.Errorf("unsupported STORAGE_TYPE %q", provider)
	}
	if err := backend.ValidateDefinition(); err != nil {
		return nil, err
	}
	return backend, nil
}

// ValidateDefinition 校验尚未绑定数据库租户 ID 的存储定义。
func (b *StorageBackend) ValidateDefinition() error {
	if b == nil || b.Name == "" {
		return fmt.Errorf("storage backend name is required")
	}
	if err := validateExternalTenantStorageProvider(b.Provider); err != nil {
		return err
	}
	if !storageallowlist.IsAllowed(b.Provider) {
		return fmt.Errorf("storage provider %q is not allowed", b.Provider)
	}
	if b.Source != StorageBackendSourceEnv {
		return fmt.Errorf("storage backend source must be env")
	}
	if b.Status != StorageBackendStatusActive {
		return fmt.Errorf("storage backend status must be active")
	}
	return b.Config.ValidateForProvider(b.Provider)
}

func validateExternalTenantStorageProvider(provider string) error {
	switch provider {
	case "local", "minio", "cos", "tos", "s3", "oss", "ks3", "obs":
		return nil
	case "":
		return fmt.Errorf("STORAGE_TYPE is required")
	default:
		return fmt.Errorf("unsupported STORAGE_TYPE %q", provider)
	}
}
