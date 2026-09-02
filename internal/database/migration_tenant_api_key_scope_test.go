package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSQLiteTenantAPIKeyScopeMigrationPreservesRows(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	versionTwelveRoot := copySQLiteMigrationsV12(t, repoRoot)
	chdirAndRestore(t, versionTwelveRoot)

	dbPath := filepath.Join(t.TempDir(), "scope-upgrade.db")
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
	db := openSQLiteDB(t, dbPath)
	version, dirty := sqliteMigrationState(t, db)
	require.Equal(t, 12, version)
	require.False(t, dirty)

	_, err := db.Exec(
		"INSERT INTO tenants (name, business) VALUES (?, ?)",
		"scope-migration-tenant", "scope-migration-test",
	)
	require.NoError(t, err)
	_, err = db.Exec(
		"INSERT INTO tenant_api_keys " +
			"(tenant_id, scope_type, name, key_hash, api_key, full_access, knowledge_base_ids, capabilities) " +
			"VALUES (1, 'tenant', 'tenant-key', 'hash-tenant', 'cipher-tenant', 0, '[\"kb-1\"]', '[\"chat\"]')",
	)
	require.NoError(t, err)
	_, err = db.Exec(
		"INSERT INTO tenant_api_keys " +
			"(scope_type, name, key_hash, api_key, full_access, capabilities) " +
			"VALUES ('platform', 'platform-key', 'hash-platform', 'cipher-platform', 0, '[\"system_tenants_read\"]')",
	)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	chdirAndRestore(t, repoRoot)
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
	db = openSQLiteDB(t, dbPath)
	version, dirty = sqliteMigrationState(t, db)
	require.Equal(t, expectedSQLiteMigrationVersion, version)
	require.False(t, dirty)
	assertSQLiteTenantAPIKeyScopeHasNoDefault(t, db)
	assertSQLiteTenantAPIKeySchemaPreserved(t, db)

	rows, err := db.Query(
		"SELECT id, tenant_id, scope_type, name, key_hash, api_key, knowledge_base_ids, capabilities " +
			"FROM tenant_api_keys ORDER BY id",
	)
	require.NoError(t, err)
	type storedKey struct {
		id               int64
		tenantID         sql.NullInt64
		scopeType        string
		name             string
		keyHash          string
		apiKey           string
		knowledgeBaseIDs string
		capabilities     string
	}
	stored := make([]storedKey, 0, 2)
	for rows.Next() {
		var key storedKey
		require.NoError(t, rows.Scan(
			&key.id, &key.tenantID, &key.scopeType, &key.name, &key.keyHash,
			&key.apiKey, &key.knowledgeBaseIDs, &key.capabilities,
		))
		stored = append(stored, key)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, []storedKey{
		{1, sql.NullInt64{Int64: 1, Valid: true}, "tenant", "tenant-key", "hash-tenant", "cipher-tenant", "[\"kb-1\"]", "[\"chat\"]"},
		{2, sql.NullInt64{}, "platform", "platform-key", "hash-platform", "cipher-platform", "[]", "[\"system_tenants_read\"]"},
	}, stored)

	_, err = db.Exec(
		"INSERT INTO tenant_api_keys (tenant_id, name, key_hash) VALUES (1, 'missing-scope', 'hash-missing')",
	)
	require.Error(t, err)
	result, err := db.Exec(
		"INSERT INTO tenant_api_keys (tenant_id, scope_type, name, key_hash) " +
			"VALUES (1, 'tenant', 'explicit-scope', 'hash-explicit')",
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)
	require.EqualValues(t, 3, insertedID)
}

func assertSQLiteTenantAPIKeySchemaPreserved(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(
		"SELECT name FROM sqlite_master " +
			"WHERE type = 'index' AND tbl_name = 'tenant_api_keys' AND sql IS NOT NULL ORDER BY name",
	)
	require.NoError(t, err)
	indexes := make([]string, 0, 4)
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		indexes = append(indexes, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, []string{
		"idx_tenant_api_keys_external_ref",
		"idx_tenant_api_keys_revoked_at",
		"idx_tenant_api_keys_scope_type",
		"idx_tenant_api_keys_tenant",
	}, indexes)

	_, err = db.Exec(
		"INSERT INTO tenant_api_keys (scope_type, name, key_hash, full_access) " +
			"VALUES ('platform', 'invalid-platform-key', 'hash-invalid-platform', 1)",
	)
	require.Error(t, err)
	_, err = db.Exec(
		"INSERT INTO tenant_api_keys (scope_type, name, key_hash) " +
			"VALUES ('tenant', 'unbound-tenant-key', 'hash-unbound-tenant')",
	)
	require.Error(t, err)
	_, err = db.Exec(
		"INSERT INTO tenant_api_keys (scope_type, name, key_hash) " +
			"VALUES ('platform', 'duplicate-key', 'hash-platform')",
	)
	require.Error(t, err)

	foreignKeys, err := db.Query("PRAGMA foreign_key_list(tenant_api_keys)")
	require.NoError(t, err)
	foundTenantReference := false
	for foreignKeys.Next() {
		var id, sequence int
		var table, from, to, onUpdate, onDelete, match string
		require.NoError(t, foreignKeys.Scan(
			&id, &sequence, &table, &from, &to, &onUpdate, &onDelete, &match,
		))
		if table == "tenants" && from == "tenant_id" && to == "id" {
			foundTenantReference = true
			require.Equal(t, "CASCADE", onDelete)
		}
	}
	require.NoError(t, foreignKeys.Err())
	require.NoError(t, foreignKeys.Close())
	require.True(t, foundTenantReference)
}

func assertSQLiteTenantAPIKeyScopeHasNoDefault(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(tenant_api_keys)")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	foundScope := false
	for rows.Next() {
		var columnID, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		require.NoError(t, rows.Scan(
			&columnID, &name, &columnType, &notNull, &defaultValue, &primaryKey,
		))
		if name == "scope_type" {
			foundScope = true
			require.Equal(t, 1, notNull)
			require.Nil(t, defaultValue)
		}
	}
	require.NoError(t, rows.Err())
	require.True(t, foundScope)
}

func copySQLiteMigrationsV12(t *testing.T, repoRoot string) string {
	t.Helper()
	destinationRoot := t.TempDir()
	sourceDir := filepath.Join(repoRoot, "migrations", "sqlite")
	destinationDir := filepath.Join(destinationRoot, "migrations", "sqlite")
	require.NoError(t, os.MkdirAll(destinationDir, 0o755))

	files := []string{
		"000000_init.up.sql",
		"000001_remove_wiki_log.up.sql",
		"000002_knowledge_folder_path.up.sql",
		"000003_knowledge_base_auto_tag_config.up.sql",
		"000004_memory.up.sql",
		"000005_messages_attachments_and_invitation_fields.up.sql",
		"000006_task_queue_and_dead_letters.up.sql",
		"000007_system_admin_and_settings.up.sql",
		"000008_processing_spans_and_pending_subtasks.up.sql",
		"000009_embed_channel_memory_flag.up.sql",
		"000010_knowledge_multi_tags.up.sql",
		"000011_principal_model.up.sql",
		"000012_message_usage.up.sql",
	}
	for _, name := range files {
		contents, err := os.ReadFile(filepath.Join(sourceDir, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(destinationDir, name), contents, 0o600))
	}
	return destinationRoot
}
