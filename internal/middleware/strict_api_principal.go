package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// StrictAPIPrincipal 在认证完成后校验 API Key 的终端主体类型，不允许隐式降级。
func StrictAPIPrincipal() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		scope, isAPIKey := types.TenantAPIKeyScopeFromContext(ctx)
		if !isAPIKey {
			c.Next()
			return
		}
		principal, hasPrincipal := types.PrincipalFromContext(ctx)
		if !hasPrincipal || !strictAPIPrincipalValid(ctx, scope, principal) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized: invalid external principal configuration",
			})
			return
		}
		c.Next()
	}
}

func strictAPIPrincipalValid(
	ctx context.Context,
	scope types.TenantAPIKeyScope,
	principal types.Principal,
) bool {
	if scope.KeyID == 0 {
		return false
	}
	switch scope.ScopeType {
	case types.APIKeyScopePlatform:
		return principal.Type == types.PrincipalAPIPlatform &&
			principal.ID == strconv.FormatUint(scope.KeyID, 10)
	case types.APIKeyScopeTenant:
		// 租户主体还必须满足当前租户声明的精确映射模式。
	default:
		return false
	}
	tenant, ok := types.TenantInfoFromContext(ctx)
	if !ok || tenant.ID == 0 || tenant.APIPrincipalConfig == nil {
		return false
	}
	switch tenant.APIPrincipalConfig.Mode {
	case types.APIPrincipalModeTenant:
		return principal.Type == types.PrincipalAPITenant &&
			principal.ID == strconv.FormatUint(tenant.ID, 10)
	case types.APIPrincipalModeDirect:
		return principal.Type == types.PrincipalAPIExternalUser &&
			externalPrincipalMatchesTenant(principal.ID, tenant.ID)
	case types.APIPrincipalModeSignedToken:
		return tenant.APIPrincipalConfig.HMACSecret != "" &&
			principal.Type == types.PrincipalAPIExternalUser &&
			externalPrincipalMatchesTenant(principal.ID, tenant.ID)
	default:
		return false
	}
}

// externalPrincipalMatchesTenant 解析“租户 ID:外部用户 ID”结构并校验租户边界。
func externalPrincipalMatchesTenant(principalID string, tenantID uint64) bool {
	tenantValue, externalUserID, ok := strings.Cut(principalID, ":")
	if !ok || externalUserID == "" {
		return false
	}
	parsedTenantID, err := strconv.ParseUint(tenantValue, 10, 64)
	return err == nil && parsedTenantID == tenantID
}
