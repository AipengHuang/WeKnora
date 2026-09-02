package repository

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
	"github.com/stretchr/testify/require"
)

const tenantPrincipalConfigTestAESKey = "0123456789abcdef0123456789abcdef"

// TestTenantRepositoryAPIPrincipalConfigPersistence 验证真实仓储更新链严格加密并拒绝明文读取。
func TestTenantRepositoryAPIPrincipalConfigPersistence(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTenantRepository(db)
	ctx := context.Background()
	tenant := &types.Tenant{Name: "principal-protocol", Status: "active"}
	require.NoError(t, repo.CreateTenant(ctx, tenant))

	t.Setenv("SYSTEM_AES_KEY", "")
	tenant.APIPrincipalConfig = &types.APIPrincipalConfig{
		Mode:       types.APIPrincipalModeSignedToken,
		HMACSecret: "test-signing-secret",
	}
	require.Error(t, repo.UpdateTenant(ctx, tenant))

	var failedValue *string
	require.NoError(t, db.Raw(
		"SELECT api_principal_config FROM tenants WHERE id = ?", tenant.ID,
	).Scan(&failedValue).Error)
	require.Nil(t, failedValue)

	t.Setenv("SYSTEM_AES_KEY", tenantPrincipalConfigTestAESKey)
	require.NoError(t, repo.UpdateTenant(ctx, tenant))
	var storedValue string
	require.NoError(t, db.Raw(
		"SELECT api_principal_config FROM tenants WHERE id = ?", tenant.ID,
	).Scan(&storedValue).Error)
	var storedConfig struct {
		HMACSecret string `json:"hmac_secret"`
	}
	require.NoError(t, json.Unmarshal([]byte(storedValue), &storedConfig))
	require.NotEqual(t, "test-signing-secret", storedConfig.HMACSecret)
	decrypted, err := utils.DecryptEncryptedStoredSecret(storedConfig.HMACSecret)
	require.NoError(t, err)
	require.Equal(t, "test-signing-secret", decrypted)

	loaded, err := repo.GetTenantByID(ctx, tenant.ID)
	require.NoError(t, err)
	require.Equal(t, "test-signing-secret", loaded.APIPrincipalConfig.HMACSecret)

	plaintext := `{"mode":"signed_token","hmac_secret":"plaintext-signing-secret"}`
	require.NoError(t, db.Exec(
		"UPDATE tenants SET api_principal_config = ? WHERE id = ?", plaintext, tenant.ID,
	).Error)
	_, err = repo.GetTenantByID(ctx, tenant.ID)
	require.True(t, stderrors.Is(err, utils.ErrStoredSecretEnvelopeRequired))
}
