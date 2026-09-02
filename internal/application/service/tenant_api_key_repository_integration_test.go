package service

import (
	"context"
	"testing"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestTenantAPIKeyServiceRepositorySecretBoundary 验证真实仓储链只在创建响应返回一次性令牌。
func TestTenantAPIKeyServiceRepositorySecretBoundary(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "0123456789abcdef0123456789abcdef")
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&types.TenantAPIKey{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	svc := NewTenantAPIKeyService(apprepo.NewTenantAPIKeyRepository(db))
	ctx := context.Background()
	created, err := svc.CreateAPIKey(ctx, interfaces.TenantAPIKeyCreateRequest{
		TenantID: 42, ScopeType: types.APIKeyScopeTenant, Name: "runtime", Capabilities: []string{"chat"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	if created.Token == "" || created.APIKey.APIKey != created.Token {
		t.Fatal("create response does not contain the one-time token")
	}

	var stored string
	if err := db.Raw("SELECT api_key FROM tenant_api_keys WHERE id = ?", created.APIKey.ID).Scan(&stored).Error; err != nil {
		t.Fatalf("read stored API key: %v", err)
	}
	decrypted, err := utils.DecryptEncryptedStoredSecret(stored)
	if err != nil || decrypted != created.Token {
		t.Fatal("stored API key is not a valid encrypted envelope")
	}

	listed, err := svc.ListAPIKeys(ctx, 42)
	if err != nil {
		t.Fatalf("ListAPIKeys returned error: %v", err)
	}
	if len(listed) != 1 || listed[0].APIKey != "" {
		t.Fatal("list response contains a reusable API key")
	}
	updated, err := svc.UpdateAPIKey(ctx, interfaces.TenantAPIKeyUpdateRequest{
		TenantID: 42, APIKeyID: created.APIKey.ID, Name: "runtime-updated", Capabilities: []string{"chat"},
	})
	if err != nil {
		t.Fatalf("UpdateAPIKey returned error: %v", err)
	}
	if updated.APIKey != "" {
		t.Fatal("update response contains a reusable API key")
	}
}
