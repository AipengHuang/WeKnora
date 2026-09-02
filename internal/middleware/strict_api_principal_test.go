package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestStrictAPIPrincipalProtocol(t *testing.T) {
	tenantMode := &types.Tenant{ID: 7, APIPrincipalConfig: &types.APIPrincipalConfig{
		Mode: types.APIPrincipalModeTenant,
	}}
	directMode := &types.Tenant{ID: 7, APIPrincipalConfig: &types.APIPrincipalConfig{
		Mode: types.APIPrincipalModeDirect,
	}}
	signedMode := &types.Tenant{ID: 7, APIPrincipalConfig: &types.APIPrincipalConfig{
		Mode: types.APIPrincipalModeSignedToken, HMACSecret: "test-secret",
	}}
	tenantScope := &types.TenantAPIKeyScope{KeyID: 9, ScopeType: types.APIKeyScopeTenant}
	platformScope := &types.TenantAPIKeyScope{KeyID: 9, ScopeType: types.APIKeyScopePlatform}
	unknownScope := &types.TenantAPIKeyScope{KeyID: 9, ScopeType: types.APIKeyScopeType("unknown")}
	missingKeyIDScope := &types.TenantAPIKeyScope{ScopeType: types.APIKeyScopeTenant}
	tests := []struct {
		name      string
		tenant    *types.Tenant
		scope     *types.TenantAPIKeyScope
		principal types.Principal
		allowed   bool
	}{
		{name: "web user", principal: types.Principal{Type: types.PrincipalWebUser, ID: "user-1"}, allowed: true},
		{name: "platform key", scope: platformScope,
			principal: types.Principal{Type: types.PrincipalAPIPlatform, ID: "9"}, allowed: true},
		{name: "tenant mode", tenant: tenantMode, scope: tenantScope,
			principal: types.Principal{Type: types.PrincipalAPITenant, ID: "7"}, allowed: true},
		{name: "direct mode", tenant: directMode, scope: tenantScope,
			principal: types.Principal{Type: types.PrincipalAPIExternalUser, ID: "7:user-1"}, allowed: true},
		{name: "signed mode", tenant: signedMode, scope: tenantScope,
			principal: types.Principal{Type: types.PrincipalAPIExternalUser, ID: "7:user-1"}, allowed: true},
		{name: "missing tenant", scope: tenantScope,
			principal: types.Principal{Type: types.PrincipalAPITenant, ID: "7"}},
		{name: "missing config", tenant: &types.Tenant{ID: 7}, scope: tenantScope,
			principal: types.Principal{Type: types.PrincipalAPITenant, ID: "7"}},
		{name: "empty mode", tenant: &types.Tenant{ID: 7, APIPrincipalConfig: &types.APIPrincipalConfig{}}, scope: tenantScope,
			principal: types.Principal{Type: types.PrincipalAPITenant, ID: "7"}},
		{name: "unknown mode", tenant: &types.Tenant{ID: 7, APIPrincipalConfig: &types.APIPrincipalConfig{
			Mode: types.APIPrincipalMode("unknown"),
		}}, scope: tenantScope, principal: types.Principal{Type: types.PrincipalAPITenant, ID: "7"}},
		{name: "direct downgrade", tenant: directMode, scope: tenantScope,
			principal: types.Principal{Type: types.PrincipalAPITenant, ID: "7"}},
		{name: "signed downgrade", tenant: signedMode, scope: tenantScope,
			principal: types.Principal{Type: types.PrincipalAPITenant, ID: "7"}},
		{name: "signed missing secret", tenant: &types.Tenant{ID: 7, APIPrincipalConfig: &types.APIPrincipalConfig{
			Mode: types.APIPrincipalModeSignedToken,
		}}, scope: tenantScope, principal: types.Principal{Type: types.PrincipalAPIExternalUser, ID: "7:user-1"}},
		{name: "external wrong tenant", tenant: directMode, scope: tenantScope,
			principal: types.Principal{Type: types.PrincipalAPIExternalUser, ID: "8:user-1"}},
		{name: "platform wrong principal", scope: platformScope,
			principal: types.Principal{Type: types.PrincipalAPITenant, ID: "7"}},
		{name: "platform wrong key", scope: platformScope,
			principal: types.Principal{Type: types.PrincipalAPIPlatform, ID: "10"}},
		{name: "unknown scope", tenant: tenantMode, scope: unknownScope,
			principal: types.Principal{Type: types.PrincipalAPITenant, ID: "7"}},
		{name: "missing key id", tenant: tenantMode, scope: missingKeyIDScope,
			principal: types.Principal{Type: types.PrincipalAPITenant, ID: "7"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, reached := runStrictAPIPrincipal(t, test.tenant, test.scope, test.principal)
			if test.allowed && (status != http.StatusNoContent || !reached) {
				t.Fatalf("status = %d, reached=%v, want 204 and true", status, reached)
			}
			if !test.allowed && (status != http.StatusUnauthorized || reached) {
				t.Fatalf("status = %d, reached=%v, want 401 and false", status, reached)
			}
		})
	}
}

func runStrictAPIPrincipal(
	t *testing.T,
	tenant *types.Tenant,
	scope *types.TenantAPIKeyScope,
	principal types.Principal,
) (int, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		ctx := types.WithPrincipal(c.Request.Context(), principal)
		if tenant != nil {
			ctx = context.WithValue(ctx, types.TenantInfoContextKey, tenant)
		}
		if scope != nil {
			ctx = types.WithTenantAPIKeyScope(ctx, *scope)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	engine.Use(StrictAPIPrincipal())
	reached := false
	engine.GET("/probe", func(c *gin.Context) {
		reached = true
		c.Status(http.StatusNoContent)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
	return response.Code, reached
}
