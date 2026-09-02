package middleware

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// 无需认证的API列表
var noAuthAPI = map[string][]string{
	"/health":                 {"GET"},
	"/api/v1/auth/register":   {"POST"},
	"/api/v1/auth/login":      {"POST"},
	"/api/v1/auth/auto-setup": {"POST"},
	// Share-link surfaces accept a plaintext invite token from anonymous
	// callers (an invitee who hasn't registered yet). They are registered
	// as public routes in RegisterAuthRoutes and rate-limited by IP, so the
	// global Auth middleware must let them through — otherwise opening a
	// share link while logged out 401s and the frontend bounces the user to
	// /login instead of the register page (issue #1617).
	"/api/v1/auth/invitations/lookup": {"POST"},
	"/api/v1/auth/register-by-invite": {"POST"},
	"/api/v1/auth/config":             {"GET"},
	"/api/v1/auth/oidc/config":        {"GET"},
	"/api/v1/auth/oidc/url":           {"GET"},
	"/api/v1/auth/oidc/start":         {"GET"},
	"/api/v1/auth/oidc/callback":      {"GET"},
	// MCP OAuth provider redirect: the third-party authorization server
	// redirects the browser here without a WeKnora bearer token. The request
	// is authenticated by the opaque, single-use `state` parameter instead.
	"/api/v1/mcp-oauth/callback": {"GET"},
	"/api/v1/auth/refresh":       {"POST"},
	// IM platforms (Feishu, Slack, etc.) commonly issue a HEAD request
	// before GET to validate Content-Type / Content-Length when rendering
	// image previews — both verbs must be allowed for image links to work.
	"/api/v1/files/presigned": {"GET", "HEAD"},
}

// 检查请求是否在无需认证的API列表中
func isNoAuthAPI(path string, method string) bool {
	for api, methods := range noAuthAPI {
		// 如果以*结尾，按照前缀匹配，否则按照全路径匹配
		if strings.HasSuffix(api, "*") {
			if strings.HasPrefix(path, strings.TrimSuffix(api, "*")) && slices.Contains(methods, method) {
				return true
			}
		} else if path == api && slices.Contains(methods, method) {
			return true
		}
	}
	return false
}

// isTenantOptionalAPI lists authenticated identity-level operations that are
// meaningful before a user belongs to any tenant. Every other authenticated
// route remains tenant-scoped and returns TENANT_REQUIRED when the JWT and
// request headers do not resolve a tenant.
func isTenantOptionalAPI(path, method string) bool {
	switch {
	case path == "/api/v1/auth/me" && (method == http.MethodGet || method == http.MethodPut):
		return true
	case path == "/api/v1/auth/me/preferences" && method == http.MethodPut:
		return true
	case path == "/api/v1/auth/logout" && method == http.MethodPost:
		return true
	case path == "/api/v1/auth/change-password" && method == http.MethodPost:
		return true
	case path == "/api/v1/auth/validate" && method == http.MethodGet:
		return true
	case path == "/api/v1/auth/switch-tenant" && method == http.MethodPost:
		return true
	case path == "/api/v1/tenants" && method == http.MethodPost:
		return true
	case strings.HasPrefix(path, "/api/v1/me/invitations"):
		return true
	default:
		return false
	}
}

func attachTenantlessUserContext(c *gin.Context, user *types.User) {
	applyAuthSession(c, authSession{
		User:        user,
		Principal:   types.Principal{Type: types.PrincipalWebUser, ID: user.ID},
		SystemAdmin: user.IsSystemAdmin,
	})
}

// Auth 认证中间件。按顺序尝试三条通道：
//
//  1. 白名单（isNoAuthAPI）/ OPTIONS 预检 —— 直接放行；
//  2. Bearer JWT —— 成功则走 authenticateJWTUser 完成空间/角色解析；
//     校验失败不立即拒绝，继续尝试 X-API-Key（保持既有兼容行为：
//     携带过期 JWT 但同时带有效 API key 的客户端仍可通过）；
//  3. X-API-Key —— authenticateAPIKeyRequest。
//
// 三条通道都未命中时返回 401；若调用方提交过 Bearer token，错误消息
// 明确指出 token 无效而不是笼统的 "missing authentication"，方便客户端
// 区分「没登录」和「登录态过期」。
func Auth(
	tenantService interfaces.TenantService,
	userService interfaces.UserService,
	memberService interfaces.TenantMemberService,
	apiKeyService interfaces.TenantAPIKeyService,
	cfg *config.Config,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ignore OPTIONS request
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		// 检查请求是否在无需认证的API列表中
		if isNoAuthAPI(c.Request.URL.Path, c.Request.Method) {
			c.Next()
			return
		}

		// 尝试JWT Token认证
		bearerPresented := false
		if token, ok := bearerToken(c); ok {
			bearerPresented = true
			user, jwtTenantID, err := userService.ValidateToken(c.Request.Context(), token)
			if err == nil && user != nil {
				if authenticateJWTUser(c, tenantService, memberService, cfg, user, jwtTenantID) {
					c.Next()
				}
				return
			}
			logger.Warnf(c.Request.Context(), "[auth] bearer token rejected: %v", err)
		}

		// 尝试X-API-Key认证（兼容模式）
		if apiKey := c.GetHeader("X-API-Key"); apiKey != "" {
			if apiKeyService == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: API key service is not configured"})
				c.Abort()
				return
			}
			if authenticateAPIKeyRequest(c, tenantService, userService, apiKeyService, apiKey) {
				c.Next()
			}
			return
		}

		// 没有任何通道认证成功
		if bearerPresented {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: invalid or expired token"})
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: missing authentication"})
		}
		c.Abort()
	}
}

// bearerToken extracts the Bearer token from the Authorization header.
func bearerToken(c *gin.Context) (string, bool) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return "", false
	}
	return strings.TrimPrefix(authHeader, "Bearer "), true
}

// authenticateJWTUser finishes authentication for a validated JWT user:
// it resolves the target tenant (X-Tenant-ID switch / JWT claim / first
// active membership), resolves the caller's role inside that tenant, and
// attaches the session context. Returns true when the request may proceed;
// on false the response has already been written and the request aborted.
func authenticateJWTUser(
	c *gin.Context,
	tenantService interfaces.TenantService,
	memberService interfaces.TenantMemberService,
	cfg *config.Config,
	user *types.User,
	jwtTenantID uint64,
) bool {
	ctx := c.Request.Context()

	targetTenantID, tenant, crossTenantSwitch, ok := resolveTargetTenant(c, tenantService, memberService, cfg, user, jwtTenantID)
	if !ok {
		return false
	}

	if targetTenantID == 0 {
		// 无可用空间：身份级路由（/auth/me 等）放行为 tenantless 会话，
		// 其余路由返回 TENANT_REQUIRED 让前端引导用户创建/加入空间。
		if isTenantOptionalAPI(c.Request.URL.Path, c.Request.Method) {
			attachTenantlessUserContext(c, user)
			return true
		}
		c.JSON(http.StatusConflict, gin.H{
			"error": "Workspace required",
			"code":  "TENANT_REQUIRED",
		})
		c.Abort()
		return false
	}

	// 获取空间信息（X-Tenant-ID 切换路径已在 resolveTargetTenant 内取到，
	// 避免二次查库）。
	if tenant == nil {
		var err error
		tenant, err = tenantService.GetTenantByID(ctx, targetTenantID)
		if err != nil || tenant == nil {
			logger.Warnf(ctx, "[auth] tenant lookup failed: tenant=%d user=%s err=%v", targetTenantID, user.ID, err)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized: invalid workspace",
			})
			c.Abort()
			return false
		}
	}

	// 解析当前空间内的角色 (issue #1303)
	role, ok := resolveTenantRole(ctx, memberService, user, targetTenantID, crossTenantSwitch, cfg)
	if !ok {
		// 强制 RBAC 时，缺少 active membership 即拒绝；fail-open 路径已在
		// resolveTenantRole 内部处理。
		logger.Warnf(ctx, "User %s has no active membership in tenant %d", user.ID, targetTenantID)
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Forbidden: not a member of the target workspace",
		})
		c.Abort()
		return false
	}

	logger.Infof(ctx,
		"[auth] resolved role=%s for user=%s in tenant=%d (jwt_tenant=%d, header=%q, cross_switch=%v)",
		role, user.ID, targetTenantID, jwtTenantID, c.GetHeader("X-Tenant-ID"), crossTenantSwitch)
	applyAuthSession(c, authSession{
		User:        user,
		Principal:   types.Principal{Type: types.PrincipalWebUser, ID: user.ID},
		TenantID:    targetTenantID,
		Tenant:      tenant,
		Role:        role,
		SystemAdmin: user.IsSystemAdmin,
	})
	return true
}

// resolveTargetTenant decides which tenant this request operates in.
//
// Priority:
//  1. X-Tenant-ID header — must parse to a positive integer, the user must
//     be allowed to access it (home tenant / cross-tenant superuser / active
//     membership, see IsTenantAccessible) and the tenant must exist. The
//     fetched tenant is returned so the caller doesn't refetch it.
//  2. JWT tenant claim (falling back to user.TenantID when the claim is 0).
//  3. First active membership — lets a tenantless session become usable as
//     soon as an invitation is accepted (see resolveFirstMembershipTarget).
//
// Returns ok=false when the response has already been written (malformed
// header, inaccessible or missing target tenant). targetTenantID == 0 with
// ok=true means "authenticated but no usable workspace" — the caller decides
// between tenantless routes and TENANT_REQUIRED.
func resolveTargetTenant(
	c *gin.Context,
	tenantService interfaces.TenantService,
	memberService interfaces.TenantMemberService,
	cfg *config.Config,
	user *types.User,
	jwtTenantID uint64,
) (targetTenantID uint64, tenant *types.Tenant, crossTenantSwitch bool, ok bool) {
	ctx := c.Request.Context()

	// 默认 target = JWT 里的 tenant_id（来自登录或 /auth/switch-tenant），
	// 兼容 ValidateToken 的 fallback：claim 缺失时 jwtTenantID == user.TenantID。
	targetTenantID = jwtTenantID
	if targetTenantID == 0 {
		targetTenantID = user.TenantID
	}

	if tenantHeader := c.GetHeader("X-Tenant-ID"); tenantHeader != "" {
		// 解析目标空间ID。畸形 / 零值必须显式拒绝：静默忽略会让坏掉的
		// 前端/SDK 悄悄写错空间，反而看不到问题。与 RequirePathTenantMatch
		// 中对 :id 的校验保持一致（非空、可解析、>0）。
		parsedTenantID, err := strconv.ParseUint(tenantHeader, 10, 64)
		if err != nil || parsedTenantID == 0 {
			logger.Warnf(ctx, "Invalid X-Tenant-ID header from user=%s: %q (err=%v)", user.ID, tenantHeader, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid X-Tenant-ID header"})
			c.Abort()
			return 0, nil, false, false
		}
		// 检查用户是否有权限访问目标空间：自家空间、跨空间超管、或
		// 有 active membership 行——三选一，由 IsTenantAccessible 统一判定。
		if !IsTenantAccessible(ctx, user, parsedTenantID, memberService, cfg) {
			logger.Warnf(ctx, "User %s attempted to access tenant %d without permission", user.ID, parsedTenantID)
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Forbidden: insufficient permissions to access target workspace",
			})
			c.Abort()
			return 0, nil, false, false
		}
		// 验证目标空间是否存在
		targetTenant, err := tenantService.GetTenantByID(ctx, parsedTenantID)
		if err != nil || targetTenant == nil {
			logger.Warnf(ctx, "Error getting target tenant by ID: %v, tenantID: %d", err, parsedTenantID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target workspace ID"})
			c.Abort()
			return 0, nil, false, false
		}
		logger.Infof(ctx, "User %s switching to tenant %d", user.ID, parsedTenantID)
		return parsedTenantID, targetTenant, parsedTenantID != user.TenantID, true
	}

	if targetTenantID == 0 {
		targetTenantID = resolveFirstMembershipTarget(ctx, user, memberService, tenantService)
	}
	return targetTenantID, nil, targetTenantID != user.TenantID, true
}

// resolveFirstMembershipTarget lets a tenantless session immediately become
// usable once an active membership exists (for example after accepting its
// first invitation or being added directly by an administrator). The user
// service persists the same earliest-membership choice on the next token
// issuance; middleware keeps the current JWT usable until then.
func resolveFirstMembershipTarget(
	ctx context.Context,
	user *types.User,
	memberService interfaces.TenantMemberService,
	tenantService interfaces.TenantService,
) uint64 {
	if user == nil || memberService == nil || tenantService == nil {
		return 0
	}
	members, err := memberService.ListByUser(ctx, user.ID)
	if err != nil {
		logger.Warnf(ctx, "Failed to list memberships for tenantless user %s: %v", user.ID, err)
		return 0
	}
	for _, member := range members {
		if member == nil || member.TenantID == 0 || member.Status != types.TenantMemberStatusActive {
			continue
		}
		tenant, err := tenantService.GetTenantByID(ctx, member.TenantID)
		if err == nil && tenant != nil {
			return member.TenantID
		}
	}
	return 0
}
