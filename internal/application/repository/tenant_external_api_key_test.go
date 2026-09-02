package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPutExternalTenantAPIKeyIsAtomicAndIdempotent(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "0123456789abcdef0123456789abcdef")
	db := openExternalAPIKeyTestDB(t)
	tenantRef := mustExternalTenantRef(t, "7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7")
	credentialRef := mustExternalCredentialRef(t, "a8af976f-47bd-5a9f-a270-4e92361e9a9d")
	createExternalTenantRow(t, db, tenantRef, "Workspace")
	repo := NewTenantAPIKeyRepository(db)

	const callers = 8
	results := make(chan *types.TenantAPIKey, callers)
	errs := make(chan error, callers)
	var wait sync.WaitGroup
	for index := range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			key := externalRuntimeAPIKey(fmt.Sprintf("hash-%d", index), fmt.Sprintf("token-%d", index))
			stored, _, err := repo.PutExternalTenantAPIKey(
				context.Background(), tenantRef, credentialRef, key,
			)
			if err != nil {
				errs <- err
				return
			}
			results <- stored
		}()
	}
	wait.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var firstID uint64
	var firstToken string
	for result := range results {
		if firstID == 0 {
			firstID = result.ID
			firstToken = result.APIKey
		}
		require.Equal(t, firstID, result.ID)
		require.Equal(t, firstToken, result.APIKey)
	}
	require.NotZero(t, firstID)
	require.NotEmpty(t, firstToken)
	var count int64
	require.NoError(t, db.Model(&types.TenantAPIKey{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	var encrypted string
	require.NoError(t, db.Raw("SELECT api_key FROM tenant_api_keys WHERE id = ?", firstID).Scan(&encrypted).Error)
	require.NotEqual(t, firstToken, encrypted)
	decrypted, err := utils.DecryptEncryptedStoredSecret(encrypted)
	require.NoError(t, err)
	require.Equal(t, firstToken, decrypted)
}

func TestPutExternalTenantAPIKeyRollsBackWithoutEncryptionKey(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "")
	db := openExternalAPIKeyTestDB(t)
	tenantRef := mustExternalTenantRef(t, "7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7")
	credentialRef := mustExternalCredentialRef(t, "a8af976f-47bd-5a9f-a270-4e92361e9a9d")
	createExternalTenantRow(t, db, tenantRef, "Workspace")
	repo := NewTenantAPIKeyRepository(db)

	_, _, err := repo.PutExternalTenantAPIKey(
		context.Background(), tenantRef, credentialRef,
		externalRuntimeAPIKey("hash-no-key", "token-no-key"),
	)
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&types.TenantAPIKey{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestPutExternalTenantAPIKeyRejectsCrossTenantCredentialReuse(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "0123456789abcdef0123456789abcdef")
	db := openExternalAPIKeyTestDB(t)
	firstTenantRef := mustExternalTenantRef(t, "7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7")
	secondTenantRef := mustExternalTenantRef(t, "c42d4e28-81f6-5f5e-b7f5-cf40f45aa864")
	credentialRef := mustExternalCredentialRef(t, "a8af976f-47bd-5a9f-a270-4e92361e9a9d")
	createExternalTenantRow(t, db, firstTenantRef, "First")
	createExternalTenantRow(t, db, secondTenantRef, "Second")
	repo := NewTenantAPIKeyRepository(db)

	_, _, err := repo.PutExternalTenantAPIKey(
		context.Background(), firstTenantRef, credentialRef,
		externalRuntimeAPIKey("hash-first", "token-first"),
	)
	require.NoError(t, err)
	_, _, err = repo.PutExternalTenantAPIKey(
		context.Background(), secondTenantRef, credentialRef,
		externalRuntimeAPIKey("hash-second", "token-second"),
	)
	require.ErrorIs(t, err, types.ErrExternalTenantCredentialConflict)
}

func openExternalAPIKeyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "external-api-key.db")
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.TenantAPIKey{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	return db
}

func createExternalTenantRow(
	t *testing.T, db *gorm.DB, ref types.ExternalTenantRef, name string,
) {
	t.Helper()
	value := ref.String()
	require.NoError(t, db.Create(&types.Tenant{ExternalRef: &value, Name: name}).Error)
}

func mustExternalTenantRef(t *testing.T, value string) types.ExternalTenantRef {
	t.Helper()
	ref, err := types.ParseExternalTenantRef(value)
	require.NoError(t, err)
	return ref
}

func mustExternalCredentialRef(t *testing.T, value string) types.ExternalTenantCredentialRef {
	t.Helper()
	ref, err := types.ParseExternalTenantCredentialRef(value)
	require.NoError(t, err)
	return ref
}

func externalRuntimeAPIKey(hash, token string) *types.TenantAPIKey {
	return &types.TenantAPIKey{
		ScopeType: types.APIKeyScopeTenant,
		Name:      "adax-web-runtime-v1",
		KeyHash:   hash,
		APIKey:    token,
		Capabilities: types.StringArray{
			string(types.APIKeyCapabilityChat),
			string(types.APIKeyCapabilityRetrieve),
			string(types.APIKeyCapabilityManageMCPServices),
		},
	}
}
