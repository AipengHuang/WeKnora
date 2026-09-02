package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExternalTenantReferenceMigrationsAreUnique(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	postgresUp, err := os.ReadFile(filepath.Join(
		repoRoot, "migrations", "versioned", "000092_external_tenant_ref.up.sql",
	))
	require.NoError(t, err)
	require.Contains(t, string(postgresUp), "external_ref UUID")
	require.Contains(t, string(postgresUp), "UNIQUE (external_ref)")

	sqliteUp, err := os.ReadFile(filepath.Join(
		repoRoot, "migrations", "sqlite", "000014_external_tenant_ref.up.sql",
	))
	require.NoError(t, err)
	require.Contains(t, string(sqliteUp), "external_ref TEXT")
	require.Contains(t, string(sqliteUp), "UNIQUE INDEX idx_tenants_external_ref")
}
