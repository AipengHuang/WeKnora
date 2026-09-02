package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTenantAPIKeyRepositoryPersistsUTCExpiry(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	t.Setenv("SYSTEM_AES_KEY", "0123456789abcdef0123456789abcdef")

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.TenantAPIKey{}))

	repo := NewTenantAPIKeyRepository(db)
	ctx := context.Background()

	expiresAt := time.Unix(time.Now().UTC().Add(5*time.Second).Unix(), 0).UTC()
	tenantID := uint64(42)
	const plaintextKey = "test-tenant-key"
	key := &types.TenantAPIKey{
		TenantID:   &tenantID,
		ScopeType:  types.APIKeyScopeTenant,
		Name:       "integration",
		KeyHash:    "hash-expiry",
		APIKey:     plaintextKey,
		FullAccess: true,
		ExpiresAt:  &expiresAt,
	}
	require.NoError(t, repo.CreateAPIKey(ctx, key))

	loaded, err := repo.GetAPIKeyByHash(ctx, key.KeyHash)
	require.NoError(t, err)
	require.NotNil(t, loaded.ExpiresAt)
	require.Empty(t, loaded.APIKey)
	require.Equal(t, time.UTC, loaded.ExpiresAt.Location())
	require.True(t, loaded.ExpiresAt.Equal(expiresAt))
	var stored string
	require.NoError(t, db.Raw("SELECT api_key FROM tenant_api_keys WHERE id = ?", key.ID).Scan(&stored).Error)
	require.NotEqual(t, plaintextKey, stored)
	decrypted, err := utils.DecryptEncryptedStoredSecret(stored)
	require.NoError(t, err)
	require.Equal(t, plaintextKey, decrypted)
}

// TestTenantAPIKeyRepositoryRejectsMissingEncryptionKey 验证缺少密钥时整次写入回滚。
func TestTenantAPIKeyRepositoryRejectsMissingEncryptionKey(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "")
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.TenantAPIKey{}))
	repo := NewTenantAPIKeyRepository(db)
	tenantID := uint64(42)
	err = repo.CreateAPIKey(context.Background(), &types.TenantAPIKey{
		TenantID:  &tenantID,
		ScopeType: types.APIKeyScopeTenant,
		Name:      "runtime",
		KeyHash:   "hash-missing-key",
		APIKey:    "test-runtime-key",
	})
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&types.TenantAPIKey{}).Count(&count).Error)
	require.Zero(t, count)
}

// TestTenantAPIKeyGORMModelRequiresExplicitScope 验证模型建表不会生成隐式租户默认值。
func TestTenantAPIKeyGORMModelRequiresExplicitScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.TenantAPIKey{}))

	columns, err := db.Migrator().ColumnTypes(&types.TenantAPIKey{})
	require.NoError(t, err)
	foundScope := false
	for _, column := range columns {
		if column.Name() != "scope_type" {
			continue
		}
		foundScope = true
		_, hasDefault := column.DefaultValue()
		require.False(t, hasDefault)
	}
	require.True(t, foundScope)

	result := db.Exec(
		"INSERT INTO tenant_api_keys (tenant_id, name, key_hash) VALUES (?, ?, ?)",
		42, "missing-scope", "hash-missing-scope",
	)
	require.Error(t, result.Error)
}

// TestTenantAPIKeyRepositoryRejectsPlaintextRead 验证明文凭据不能从数据库进入运行时。
func TestTenantAPIKeyRepositoryRejectsPlaintextRead(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "0123456789abcdef0123456789abcdef")
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.TenantAPIKey{}))
	tenantID := uint64(42)
	key := &types.TenantAPIKey{
		TenantID: &tenantID, ScopeType: types.APIKeyScopeTenant, Name: "plaintext",
		KeyHash: "hash-plaintext", APIKey: "plaintext-runtime-key",
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(key).Error)
	repo := NewTenantAPIKeyRepository(db)
	_, err = repo.GetAPIKeyByHash(context.Background(), key.KeyHash)
	require.ErrorIs(t, err, utils.ErrStoredSecretEnvelopeRequired)
}

// TestTenantAPIKeyRepositoryUpdateIsTenantScoped 验证通用更新不会越过租户边界。
// 输入同租户和其他租户的 Key；前者更新全部可配置字段，后者必须返回未找到。
func TestTenantAPIKeyRepositoryUpdateIsTenantScoped(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "0123456789abcdef0123456789abcdef")
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.TenantAPIKey{}))
	repo := NewTenantAPIKeyRepository(db)
	ctx := context.Background()
	tenant42, tenant43 := uint64(42), uint64(43)
	keys := []*types.TenantAPIKey{
		{TenantID: &tenant42, ScopeType: types.APIKeyScopeTenant, Name: "scoped", KeyHash: "hash-scoped", APIKey: "sk-scoped"},
		{TenantID: &tenant43, ScopeType: types.APIKeyScopeTenant, Name: "other", KeyHash: "hash-other", APIKey: "sk-other"},
		{TenantID: &tenant42, ScopeType: types.APIKeyScopeTenant, Name: "full", KeyHash: "hash-full", APIKey: "sk-full", FullAccess: true},
	}
	for _, key := range keys {
		require.NoError(t, repo.CreateAPIKey(ctx, key))
	}

	expiresAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	updated, err := repo.UpdateAPIKey(ctx, tenant42, keys[0].ID, &types.TenantAPIKey{
		Name: "updated", FullAccess: false,
		KnowledgeBaseIDs: types.StringArray{"kb-1", "kb-2"},
		Capabilities:     types.StringArray{"retrieve", "chat"},
		ExpiresAt:        &expiresAt,
	})
	require.NoError(t, err)
	require.Equal(t, "updated", updated.Name)
	require.Equal(t, types.StringArray{"kb-1", "kb-2"}, updated.KnowledgeBaseIDs)
	require.Equal(t, types.StringArray{"retrieve", "chat"}, updated.Capabilities)
	require.NotNil(t, updated.ExpiresAt)
	require.True(t, updated.ExpiresAt.Equal(expiresAt))

	_, err = repo.UpdateAPIKey(ctx, tenant42, keys[1].ID, &types.TenantAPIKey{Name: "blocked"})
	require.ErrorIs(t, err, ErrTenantAPIKeyNotFound)

	full, err := repo.UpdateAPIKey(ctx, tenant42, keys[2].ID, &types.TenantAPIKey{
		Name: "full updated", FullAccess: false, Capabilities: types.StringArray{"retrieve"},
	})
	require.NoError(t, err)
	require.False(t, full.FullAccess)
}
