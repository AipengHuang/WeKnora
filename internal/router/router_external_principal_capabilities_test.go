package router

import (
	"net/http"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestPlatformTenantCredentialRoutesDeclarePlatformCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := &rbacGuards{}
	v1 := gin.New().Group("/api/v1")
	RegisterTenantRoutes(
		v1,
		&handler.TenantHandler{},
		&handler.TenantMemberHandler{},
		&handler.TenantInvitationHandler{},
		nil,
		g,
	)

	cases := []struct {
		method     string
		path       string
		capability types.APIKeyCapability
	}{
		{http.MethodGet, "/api/v1/tenants/:id/api-keys", types.APIKeyCapabilitySystemTenantsRead},
		{http.MethodPost, "/api/v1/tenants/:id/api-keys", types.APIKeyCapabilitySystemTenantsManage},
		{http.MethodDelete, "/api/v1/tenants/:id/api-keys/:key_id", types.APIKeyCapabilitySystemTenantsManage},
		{http.MethodGet, "/api/v1/tenants/:id/api-principal-config", types.APIKeyCapabilitySystemTenantsRead},
		{http.MethodPut, "/api/v1/tenants/:id/api-principal-config", types.APIKeyCapabilitySystemTenantsManage},
	}
	for _, test := range cases {
		policy := mustLookupAPIKeyPolicy(t, g, test.method, test.path)
		if !policy.PlatformOnly {
			t.Fatalf("%s %s must be platform-only", test.method, test.path)
		}
		if !policyHasCapability(policy, test.capability) {
			t.Fatalf("%s %s capabilities = %#v, want %s", test.method, test.path, policy.Capabilities, test.capability)
		}
	}
	if _, ok := g.apiKeyAuthorizer.Lookup(http.MethodPut, "/api/v1/tenants/:id/api-keys/:key_id"); ok {
		t.Fatal("platform API keys must not update tenant runtime keys")
	}
}

func TestInteractiveAgentRoutesDeclareChatCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := &rbacGuards{}
	v1 := gin.New().Group("/api/v1")
	RegisterMCPServiceRoutes(
		v1,
		&handler.MCPServiceHandler{},
		&handler.MCPCredentialsHandler{},
		&handler.MCPOAuthHandler{},
		g,
	)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/agent/tool-approvals/:pending_id"},
		{http.MethodPost, "/api/v1/agent/mcp-oauth-resolutions/:pending_id"},
		{http.MethodPost, "/api/v1/agent/mcp-oauth-resolutions/:pending_id/cancel"},
	}
	for _, test := range cases {
		policy := mustLookupAPIKeyPolicy(t, g, test.method, test.path)
		if !policy.RequireFullAccess {
			t.Fatalf("%s must require full access without chat", test.path)
		}
		if !policyHasCapability(policy, types.APIKeyCapabilityChat) {
			t.Fatalf("%s capabilities = %#v, want chat", test.path, policy.Capabilities)
		}
	}
}
