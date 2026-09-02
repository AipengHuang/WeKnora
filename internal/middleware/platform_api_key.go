package middleware

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// RequirePlatformAPIKey 禁止 JWT 和租户密钥进入机器控制面端点。
func RequirePlatformAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := types.TenantAPIKeyScopeFromContext(c.Request.Context())
		if !ok || !scope.IsPlatform() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Forbidden: platform API key required",
			})
			return
		}
		c.Next()
	}
}
