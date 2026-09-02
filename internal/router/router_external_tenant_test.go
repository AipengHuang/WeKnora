package router

import (
	"net/http"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestExternalTenantRouteRequiresPlatformTenantManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	guards := &rbacGuards{}
	v1 := gin.New().Group("/api/v1")
	RegisterExternalTenantRoutes(v1, &handler.TenantHandler{}, guards)
	policy := mustLookupAPIKeyPolicy(
		t,
		guards,
		http.MethodPut,
		"/api/v1/system/admin/external-tenants/:external_ref",
	)
	if !policy.PlatformOnly {
		t.Fatal("external tenant route must be platform-only")
	}
	if !policyHasCapability(policy, types.APIKeyCapabilitySystemTenantsManage) {
		t.Fatalf("capabilities = %#v, want system_tenants_manage", policy.Capabilities)
	}
}

func TestExternalTenantAPIKeyRouteRequiresPlatformTenantManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	guards := &rbacGuards{}
	v1 := gin.New().Group("/api/v1")
	RegisterExternalTenantRoutes(v1, &handler.TenantHandler{}, guards)
	policy := mustLookupAPIKeyPolicy(
		t,
		guards,
		http.MethodPut,
		"/api/v1/system/admin/external-tenants/:external_ref/api-keys/:external_credential_ref",
	)
	if !policy.PlatformOnly {
		t.Fatal("external tenant API key route must be platform-only")
	}
	if !policyHasCapability(policy, types.APIKeyCapabilitySystemTenantsManage) {
		t.Fatalf("capabilities = %#v, want system_tenants_manage", policy.Capabilities)
	}
}
