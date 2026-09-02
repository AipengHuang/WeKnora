package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

func TestTenantAPIKeyServiceRejectsInvalidCapabilities(t *testing.T) {
	tests := []struct {
		name         string
		capabilities []string
	}{
		{name: "unknown", capabilities: []string{"bogus"}},
		{name: "whitespace alias", capabilities: []string{" chat"}},
		{name: "case alias", capabilities: []string{"CHAT"}},
		{name: "duplicate", capabilities: []string{"chat", "chat"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc := NewTenantAPIKeyService(newFakeTenantAPIKeyRepo())
			_, err := svc.CreateAPIKey(context.Background(), interfaces.TenantAPIKeyCreateRequest{
				TenantID: 42, ScopeType: types.APIKeyScopeTenant,
				Name: "invalid", Capabilities: test.capabilities,
			})
			if err == nil {
				t.Fatal("CreateAPIKey returned nil error")
			}
		})
	}
}

func TestTenantAPIKeyServiceRejectsEmptyScope(t *testing.T) {
	svc := NewTenantAPIKeyService(newFakeTenantAPIKeyRepo())
	_, err := svc.CreateAPIKey(context.Background(), interfaces.TenantAPIKeyCreateRequest{
		TenantID: 42,
		Name:     "invalid",
	})
	if err == nil {
		t.Fatal("CreateAPIKey returned nil error")
	}
}

func TestTenantAPIKeyServiceRejectsInvalidScope(t *testing.T) {
	svc := NewTenantAPIKeyService(newFakeTenantAPIKeyRepo())
	_, err := svc.CreateAPIKey(context.Background(), interfaces.TenantAPIKeyCreateRequest{
		TenantID: 42, ScopeType: types.APIKeyScopeType(" PLATFORM "), Name: "invalid",
	})
	if err == nil {
		t.Fatal("CreateAPIKey returned nil error")
	}
}

func TestTenantAPIKeyServiceUpdateRejectsDuplicateCapabilities(t *testing.T) {
	ctx := context.Background()
	repo := newFakeTenantAPIKeyRepo()
	svc := NewTenantAPIKeyService(repo)
	created, err := svc.CreateAPIKey(ctx, interfaces.TenantAPIKeyCreateRequest{
		TenantID: 42, ScopeType: types.APIKeyScopeTenant, Name: "runtime", Capabilities: []string{"chat"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	_, err = svc.UpdateAPIKey(ctx, interfaces.TenantAPIKeyUpdateRequest{
		TenantID: 42, APIKeyID: created.APIKey.ID, Name: "runtime",
		Capabilities: []string{"chat", "chat"},
	})
	if err == nil {
		t.Fatal("UpdateAPIKey returned nil error")
	}
	if got := repo.byHash[created.APIKey.KeyHash].Capabilities; len(got) != 1 || got[0] != "chat" {
		t.Fatalf("stored capabilities = %#v, want unchanged chat capability", got)
	}
}

func TestTenantAPIKeyServiceAuthenticateRejectsInvalidStoredCapabilities(t *testing.T) {
	ctx := context.Background()
	repo := newFakeTenantAPIKeyRepo()
	svc := NewTenantAPIKeyService(repo)
	created, err := svc.CreateAPIKey(ctx, interfaces.TenantAPIKeyCreateRequest{
		TenantID: 42, ScopeType: types.APIKeyScopeTenant, Name: "runtime", FullAccess: true,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	repo.byHash[created.APIKey.KeyHash].Capabilities = types.StringArray{"chat", "chat"}
	if _, err := svc.AuthenticateAPIKey(ctx, created.Token); err == nil {
		t.Fatal("AuthenticateAPIKey returned nil error")
	}
}

func TestTenantAPIKeyServiceAuthenticateRejectsInvalidStoredScope(t *testing.T) {
	ctx := context.Background()
	repo := newFakeTenantAPIKeyRepo()
	svc := NewTenantAPIKeyService(repo)
	created, err := svc.CreateAPIKey(ctx, interfaces.TenantAPIKeyCreateRequest{
		TenantID: 42, ScopeType: types.APIKeyScopeTenant, Name: "runtime", FullAccess: true,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	repo.byHash[created.APIKey.KeyHash].ScopeType = types.APIKeyScopeType("unknown")
	if _, err := svc.AuthenticateAPIKey(ctx, created.Token); err == nil {
		t.Fatal("AuthenticateAPIKey returned nil error")
	}
}

func TestTenantAPIKeyServiceAuthenticateRejectsInvalidStoredScopeInvariants(t *testing.T) {
	tests := []struct {
		name   string
		create interfaces.TenantAPIKeyCreateRequest
		mutate func(*types.TenantAPIKey)
	}{
		{
			name: "platform full access",
			create: interfaces.TenantAPIKeyCreateRequest{
				ScopeType: types.APIKeyScopePlatform, Name: "platform", Capabilities: []string{"chat"},
			},
			mutate: func(key *types.TenantAPIKey) { key.FullAccess = true },
		},
		{
			name: "platform bound to tenant",
			create: interfaces.TenantAPIKeyCreateRequest{
				ScopeType: types.APIKeyScopePlatform, Name: "platform", Capabilities: []string{"chat"},
			},
			mutate: func(key *types.TenantAPIKey) { key.TenantID = uint64Pointer(42) },
		},
		{
			name: "platform without capabilities",
			create: interfaces.TenantAPIKeyCreateRequest{
				ScopeType: types.APIKeyScopePlatform, Name: "platform", Capabilities: []string{"chat"},
			},
			mutate: func(key *types.TenantAPIKey) { key.Capabilities = nil },
		},
		{
			name: "tenant without tenant id",
			create: interfaces.TenantAPIKeyCreateRequest{
				TenantID: 42, ScopeType: types.APIKeyScopeTenant, Name: "tenant", FullAccess: true,
			},
			mutate: func(key *types.TenantAPIKey) { key.TenantID = nil },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := newFakeTenantAPIKeyRepo()
			svc := NewTenantAPIKeyService(repo)
			created, err := svc.CreateAPIKey(ctx, test.create)
			if err != nil {
				t.Fatalf("CreateAPIKey returned error: %v", err)
			}
			test.mutate(repo.byHash[created.APIKey.KeyHash])
			if _, err := svc.AuthenticateAPIKey(ctx, created.Token); err == nil {
				t.Fatal("AuthenticateAPIKey returned nil error")
			}
		})
	}
}

func TestTenantAPIKeyServiceRemovesSecretsFromNonCreateResults(t *testing.T) {
	ctx := context.Background()
	repo := newFakeTenantAPIKeyRepo()
	svc := NewTenantAPIKeyService(repo)
	created, err := svc.CreateAPIKey(ctx, interfaces.TenantAPIKeyCreateRequest{
		TenantID: 42, ScopeType: types.APIKeyScopeTenant, Name: "runtime", Capabilities: []string{"chat"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	listed, err := svc.ListAPIKeys(ctx, 42)
	if err != nil {
		t.Fatalf("ListAPIKeys returned error: %v", err)
	}
	if len(listed) != 1 || listed[0].APIKey != "" {
		t.Fatalf("listed API key secret = %q, want empty", listed[0].APIKey)
	}
	updated, err := svc.UpdateAPIKey(ctx, interfaces.TenantAPIKeyUpdateRequest{
		TenantID: 42, APIKeyID: created.APIKey.ID, Name: "runtime", Capabilities: []string{"chat"},
	})
	if err != nil {
		t.Fatalf("UpdateAPIKey returned error: %v", err)
	}
	if updated.APIKey != "" {
		t.Fatalf("updated API key secret = %q, want empty", updated.APIKey)
	}
	_, err = svc.CreateAPIKey(ctx, interfaces.TenantAPIKeyCreateRequest{
		ScopeType: types.APIKeyScopePlatform, Name: "platform", Capabilities: []string{"system_tenants_read"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey for platform key returned error: %v", err)
	}
	platformKeys, err := svc.ListPlatformAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListPlatformAPIKeys returned error: %v", err)
	}
	if len(platformKeys) != 1 || platformKeys[0].APIKey != "" {
		t.Fatalf("listed platform API key secret = %q, want empty", platformKeys[0].APIKey)
	}
}
