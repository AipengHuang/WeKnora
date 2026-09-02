package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

func authenticateAPIKeyRequest(
	c *gin.Context,
	tenantService interfaces.TenantService,
	userService interfaces.UserService,
	apiKeyService interfaces.TenantAPIKeyService,
	apiKey string,
) bool {
	ctx := c.Request.Context()
	// AuthenticateAPIKey resolves the key by SHA-256 hash (see startup
	// BackfillMissingKeyHashes for migration 000065 placeholder rows).
	key, err := apiKeyService.AuthenticateAPIKey(ctx, apiKey)
	if err != nil || key == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: invalid API key"})
		c.Abort()
		return false
	}

	if key.IsPlatform() {
		tenantHeader := strings.TrimSpace(c.GetHeader("X-Tenant-ID"))
		if tenantHeader == "" {
			if !isPlatformTenantOptionalAPI(c.FullPath(), c.Request.Method) {
				c.JSON(http.StatusConflict, gin.H{
					"error": "Workspace required: platform API keys must send X-Tenant-ID",
					"code":  "TENANT_REQUIRED",
				})
				c.Abort()
				return false
			}
			attachPlatformAPIKeyAuthContext(c, key)
		} else {
			targetTenantID, parseErr := strconv.ParseUint(tenantHeader, 10, 64)
			if parseErr != nil || targetTenantID == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid X-Tenant-ID header"})
				c.Abort()
				return false
			}
			attachAPIKeyAuthContext(c, tenantService, userService, targetTenantID, key)
		}
	} else {
		tenantID := key.TenantIDValue()
		if tenantID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: invalid API key scope"})
			c.Abort()
			return false
		}
		if tenantHeader := strings.TrimSpace(c.GetHeader("X-Tenant-ID")); tenantHeader != "" {
			requestedTenantID, parseErr := strconv.ParseUint(tenantHeader, 10, 64)
			if parseErr != nil || requestedTenantID == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid X-Tenant-ID header"})
				c.Abort()
				return false
			}
			if requestedTenantID != tenantID {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "Forbidden: workspace API key cannot switch workspaces",
				})
				c.Abort()
				return false
			}
		}
		attachAPIKeyAuthContext(c, tenantService, userService, tenantID, key)
	}
	if c.IsAborted() {
		return false
	}
	// Per-route API-key authorization (full access + capabilities + KB scope)
	// is enforced by middleware.APIKeyRouteAuthorizer on the /api/v1 group.
	// Key-management and any other undeclared route is denied there.
	return true
}

type platformTenantlessRoute struct {
	Method string
	Path   string
}

var platformTenantlessRoutes = map[platformTenantlessRoute]struct{}{
	{Method: http.MethodGet, Path: "/api/v1/tenants/all"}:                                                        {},
	{Method: http.MethodGet, Path: "/api/v1/tenants/search"}:                                                     {},
	{Method: http.MethodPost, Path: "/api/v1/tenants"}:                                                           {},
	{Method: http.MethodPut, Path: "/api/v1/system/admin/external-tenants/:external_ref"}:                        {},
	{Method: http.MethodGet, Path: "/api/v1/system/admin/settings"}:                                              {},
	{Method: http.MethodGet, Path: "/api/v1/system/admin/settings/:key"}:                                         {},
	{Method: http.MethodPut, Path: "/api/v1/system/admin/settings/:key"}:                                         {},
	{Method: http.MethodDelete, Path: "/api/v1/system/admin/settings/:key"}:                                      {},
	{Method: http.MethodGet, Path: "/api/v1/system/admin/runtime/queues"}:                                        {},
	{Method: http.MethodGet, Path: "/api/v1/system/admin/runtime/queues/:queue/tasks"}:                           {},
	{Method: http.MethodPost, Path: "/api/v1/system/admin/runtime/queues/:queue/tasks/:task_id/actions/:action"}: {},
	{Method: http.MethodDelete, Path: "/api/v1/system/admin/runtime/queues/:queue/archived"}:                     {},
	{Method: http.MethodPost, Path: "/api/v1/system/admin/tenants/apply-default-storage-quota"}:                  {},
	{Method: http.MethodGet, Path: "/api/v1/system/admin/audit-log"}:                                             {},
}

// isPlatformTenantOptionalAPI 只接受路由注册表声明的精确模板。
func isPlatformTenantOptionalAPI(path, method string) bool {
	_, ok := platformTenantlessRoutes[platformTenantlessRoute{Method: method, Path: path}]
	return ok
}

func attachPlatformAPIKeyAuthContext(c *gin.Context, key *types.TenantAPIKey) {
	principal, user := platformAPIKeyIdentity(key)
	applyAuthSession(c, authSession{
		User:      user,
		Principal: principal,
		// This role context exists only for legacy guard compatibility after
		// RequireRole short-circuits API-key principals; the key's real
		// authority is its platform capabilities enforced by the APIKeyGate.
		Role: types.TenantRoleViewer,
		APIKeyScope: &types.TenantAPIKeyScope{
			KeyID:        key.ID,
			ScopeType:    types.APIKeyScopePlatform,
			FullAccess:   false,
			Capabilities: key.Capabilities,
		},
	})
}

func platformAPIKeyIdentity(key *types.TenantAPIKey) (types.Principal, *types.User) {
	keyID := uint64(0)
	if key != nil {
		keyID = key.ID
	}
	principal := types.Principal{Type: types.PrincipalAPIPlatform, ID: strconv.FormatUint(keyID, 10)}
	userID := principal.StorageID()
	return principal, &types.User{
		ID:       userID,
		Username: userID,
		Email:    fmt.Sprintf("platform-api-key-%d@api-key.local", keyID),
		IsActive: true,
	}
}

func attachAPIKeyAuthContext(
	c *gin.Context,
	tenantService interfaces.TenantService,
	userService interfaces.UserService,
	tenantID uint64,
	key *types.TenantAPIKey,
) {
	t, err := tenantService.GetTenantByID(c.Request.Context(), tenantID)
	if err != nil {
		logger.Warnf(c.Request.Context(), "[auth] API key tenant lookup failed: tenant=%d err=%v", tenantID, err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: invalid API key"})
		c.Abort()
		return
	}

	var user *types.User
	var principal types.Principal
	if key != nil && key.IsPlatform() {
		// A platform key keeps one stable machine identity while selecting the
		// target workspace through X-Tenant-ID. Tenant API-principal modes and
		// tenant-owned synthetic users must not rewrite that identity.
		principal, user = platformAPIKeyIdentity(key)
		user.TenantID = tenantID
	} else {
		user, err = userService.GetUserByTenantID(c.Request.Context(), tenantID)
		if err != nil || user == nil {
			user = &types.User{
				ID:       fmt.Sprintf("system-%d", tenantID),
				Username: fmt.Sprintf("system-%d", tenantID),
				Email:    fmt.Sprintf("system-%d@api-key.local", tenantID),
				TenantID: tenantID,
				IsActive: true,
			}
			logger.Infof(c.Request.Context(),
				"No user found for tenant %d via API key, using synthetic system user %s", tenantID, user.ID)
		}

		var principalErr error
		principal, principalErr = resolveAPIPrincipal(c.Request.Context(), t, c.Request.Header)
		if principalErr != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": apiPrincipalAuthErrorMessage(principalErr)})
			c.Abort()
			return
		}
	}

	// This role context exists only for legacy guard compatibility after
	// RequireRole short-circuits API-key principals. The API key's real
	// authority is FullAccess + Capabilities + KnowledgeBaseIDs.
	apiKeyTenantRoleContext := types.TenantRoleViewer
	fullAccess := key != nil && key.FullAccess && !key.IsPlatform()
	if fullAccess {
		apiKeyTenantRoleContext = types.TenantRoleOwner
	}
	session := authSession{
		User:      user,
		Principal: principal,
		TenantID:  tenantID,
		Tenant:    t,
		Role:      apiKeyTenantRoleContext,
	}
	if key != nil {
		session.APIKeyScope = &types.TenantAPIKeyScope{
			KeyID:            key.ID,
			ScopeType:        key.ScopeType,
			FullAccess:       fullAccess,
			KnowledgeBaseIDs: key.KnowledgeBaseIDs,
			Capabilities:     key.Capabilities,
		}
	}
	applyAuthSession(c, session)
}
