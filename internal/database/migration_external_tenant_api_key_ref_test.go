package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExternalTenantAPIKeyReferenceMigrationsAreUnique(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	postgresUp, err := os.ReadFile(filepath.Join(
		repoRoot, "migrations", "versioned", "000093_external_tenant_api_key_ref.up.sql",
	))
	require.NoError(t, err)
	require.Contains(t, string(postgresUp), "external_ref UUID")
	require.Contains(t, string(postgresUp), "UNIQUE (external_ref)")

	sqliteUp, err := os.ReadFile(filepath.Join(
		repoRoot, "migrations", "sqlite", "000015_external_tenant_api_key_ref.up.sql",
	))
	require.NoError(t, err)
	require.Contains(t, string(sqliteUp), "external_ref TEXT")
	require.Contains(t, string(sqliteUp), "UNIQUE INDEX idx_tenant_api_keys_external_ref")
}
