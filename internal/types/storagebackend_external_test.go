package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExternalTenantStorageBackendRequiresExactProvider(t *testing.T) {
	for _, provider := range []string{"", "LOCAL", " local", "unknown"} {
		t.Run(provider, func(t *testing.T) {
			t.Setenv("STORAGE_TYPE", provider)
			_, err := ExternalTenantStorageBackendFromEnvironment(0)
			require.Error(t, err)
		})
	}
}

func TestExternalTenantStorageBackendValidatesConfiguration(t *testing.T) {
	t.Setenv("STORAGE_TYPE", "s3")
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("S3_REGION", "")
	t.Setenv("S3_ACCESS_KEY", "")
	t.Setenv("S3_SECRET_KEY", "")
	t.Setenv("S3_BUCKET_NAME", "")
	_, err := ExternalTenantStorageBackendFromEnvironment(0)
	require.Error(t, err)
}

func TestExternalTenantStorageBackendAcceptsExplicitLocal(t *testing.T) {
	t.Setenv("STORAGE_TYPE", "local")
	backend, err := ExternalTenantStorageBackendFromEnvironment(0)
	require.NoError(t, err)
	require.Equal(t, "local", backend.Provider)
	require.Equal(t, StorageBackendSourceEnv, backend.Source)
}
