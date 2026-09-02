package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// TestAPIKeyGateDeniesTenantKeyManagementPaths 保证租户密钥管理与测试令牌保持默认拒绝。
func TestAPIKeyGateDeniesTenantKeyManagementPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gate := middleware.NewAPIKeyRouteAuthorizer()
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		scope := types.TenantAPIKeyScope{FullAccess: true}
		c.Request = c.Request.WithContext(types.WithTenantAPIKeyScope(c.Request.Context(), scope))
		c.Next()
	})
	engine.Use(gate.Middleware())

	v1 := engine.Group("/api/v1")
	tenantByID := v1.Group("/tenants/:id")
	{
		tenantByID.GET("/api-keys", reachedOK)
		tenantByID.POST("/api-keys", reachedOK)
		tenantByID.PUT("/api-keys/:key_id", reachedOK)
		tenantByID.DELETE("/api-keys/:key_id", reachedOK)
		tenantByID.GET("/api-principal-config", reachedOK)
		tenantByID.PUT("/api-principal-config", reachedOK)
		tenantByID.POST("/api-principal-test-token", reachedOK)
	}
	tenantRoutes := v1.Group("/tenants")
	{
		tenantRoutes.POST("", reachedOK)
	}

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/tenants/42/api-keys"},
		{http.MethodPost, "/api/v1/tenants/42/api-keys"},
		{http.MethodPut, "/api/v1/tenants/42/api-keys/7"},
		{http.MethodDelete, "/api/v1/tenants/42/api-keys/7"},
		{http.MethodGet, "/api/v1/tenants/42/api-principal-config"},
		{http.MethodPut, "/api/v1/tenants/42/api-principal-config"},
		{http.MethodPost, "/api/v1/tenants/42/api-principal-test-token"},
		{http.MethodPost, "/api/v1/tenants"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 body=%s", w.Code, w.Body.String())
			}
		})
	}
}

// TestAPIKeyGateAllowsDeclaredTenantKVRead 保证显式声明的全权限租户路由仍可访问。
func TestAPIKeyGateAllowsDeclaredTenantKVRead(t *testing.T) {
	gin.SetMode(gin.TestMode)

	g := &rbacGuards{}
	v1 := gin.New().Group("/api/v1")
	g.apiKeyRoute(v1.Group("/tenants"), http.MethodGet, "/kv/:key", apiKeyFullAccess(), reachedOK)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		scope := types.TenantAPIKeyScope{FullAccess: true}
		c.Request = c.Request.WithContext(types.WithTenantAPIKeyScope(c.Request.Context(), scope))
		c.Next()
	})
	engine.Use(g.ensureAPIKeyAuthorizer().Middleware())
	engine.GET("/api/v1/tenants/kv/:key", reachedOK)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/tenants/kv/my-key", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("declared owner route status = %d, want 200 body=%s", w.Code, w.Body.String())
	}
}

func TestAPIKeyGateAllowsExactPlatformTenantKeyLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)

	g := &rbacGuards{}
	policies := gin.New().Group("/api/v1/tenants/:id")
	g.apiKeyRoute(policies, http.MethodGet, "/api-keys",
		apiKeyPlatform(types.APIKeyCapabilitySystemTenantsRead), reachedOK)
	g.apiKeyRoute(policies, http.MethodPost, "/api-keys",
		apiKeyPlatform(types.APIKeyCapabilitySystemTenantsManage), reachedOK)
	g.apiKeyRoute(policies, http.MethodDelete, "/api-keys/:key_id",
		apiKeyPlatform(types.APIKeyCapabilitySystemTenantsManage), reachedOK)

	cases := []struct {
		method     string
		pattern    string
		path       string
		capability types.APIKeyCapability
	}{
		{http.MethodGet, "/api/v1/tenants/:id/api-keys", "/api/v1/tenants/42/api-keys", types.APIKeyCapabilitySystemTenantsRead},
		{http.MethodPost, "/api/v1/tenants/:id/api-keys", "/api/v1/tenants/42/api-keys", types.APIKeyCapabilitySystemTenantsManage},
		{http.MethodDelete, "/api/v1/tenants/:id/api-keys/:key_id", "/api/v1/tenants/42/api-keys/7", types.APIKeyCapabilitySystemTenantsManage},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				scope := types.TenantAPIKeyScope{
					ScopeType:    types.APIKeyScopePlatform,
					Capabilities: types.StringArray{string(tc.capability)},
				}
				c.Request = c.Request.WithContext(
					types.WithTenantAPIKeyScope(c.Request.Context(), scope),
				)
				c.Next()
			})
			engine.Use(g.apiKeyAuthorizer.Middleware())
			engine.Handle(tc.method, tc.pattern, reachedOK)

			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAPIKeyGateRejectsPlatformTenantKeyUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	g := &rbacGuards{}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		scope := types.TenantAPIKeyScope{
			ScopeType: types.APIKeyScopePlatform,
			Capabilities: types.StringArray{
				string(types.APIKeyCapabilitySystemTenantsManage),
			},
		}
		c.Request = c.Request.WithContext(
			types.WithTenantAPIKeyScope(c.Request.Context(), scope),
		)
		c.Next()
	})
	engine.Use(g.ensureAPIKeyAuthorizer().Middleware())
	engine.PUT("/api/v1/tenants/:id/api-keys/:key_id", reachedOK)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(
		http.MethodPut,
		"/api/v1/tenants/42/api-keys/7",
		nil,
	))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 body=%s", w.Code, w.Body.String())
	}
}

// TestPlatformTenantRouteRunsFullAuthChain 验证真实中间件链解析目标租户并到达处理器。
func TestPlatformTenantRouteRunsFullAuthChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := &types.TenantAPIKey{
		ID:        9,
		ScopeType: types.APIKeyScopePlatform,
		Capabilities: types.StringArray{
			string(types.APIKeyCapabilitySystemTenantsRead),
		},
	}
	tenantService := &routeAuthTenantService{tenant: &types.Tenant{ID: 42}}
	apiKeyService := &routeAuthAPIKeyService{key: key}
	g := &rbacGuards{}
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(middleware.Auth(tenantService, nil, nil, apiKeyService, nil))
	engine.Use(middleware.StrictAPIPrincipal())
	v1 := engine.Group("/api/v1")
	v1.Use(g.ensureAPIKeyAuthorizer().Middleware())
	tenantByID := v1.Group("/tenants/:id", g.PathTenantMatch())
	reached := false
	g.apiKeyRoute(tenantByID, http.MethodGet, "/api-keys",
		apiKeyPlatform(types.APIKeyCapabilitySystemTenantsRead), g.Owner(), func(c *gin.Context) {
			tenantID, tenantOK := types.TenantIDFromContext(c.Request.Context())
			principal, principalOK := types.PrincipalFromContext(c.Request.Context())
			reached = tenantOK && tenantID == 42 && principalOK && principal.Type == types.PrincipalAPIPlatform
			c.Status(http.StatusOK)
		})

	request := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-API-Key", "test-platform-key")
		req.Header.Set("X-Tenant-ID", "42")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}

	response := request("/api/v1/tenants/42/api-keys")
	if response.Code != http.StatusOK || !reached {
		t.Fatalf("matching route status = %d, reached=%v", response.Code, reached)
	}
	reached = false
	response = request("/api/v1/tenants/43/api-keys")
	if response.Code != http.StatusForbidden || reached {
		t.Fatalf("mismatched route status = %d, reached=%v", response.Code, reached)
	}
}

type routeAuthTenantService struct {
	interfaces.TenantService
	tenant *types.Tenant
}

func (s *routeAuthTenantService) GetTenantByID(context.Context, uint64) (*types.Tenant, error) {
	return s.tenant, nil
}

type routeAuthAPIKeyService struct {
	interfaces.TenantAPIKeyService
	key *types.TenantAPIKey
}

func (s *routeAuthAPIKeyService) AuthenticateAPIKey(context.Context, string) (*types.TenantAPIKey, error) {
	return s.key, nil
}

func reachedOK(c *gin.Context) {
	c.Status(http.StatusOK)
}
