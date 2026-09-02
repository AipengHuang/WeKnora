package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequirePlatformAPIKeyAcceptsOnlyPlatformScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		scope  *types.TenantAPIKeyScope
		status int
	}{
		{"jwt", nil, http.StatusForbidden},
		{"tenant", &types.TenantAPIKeyScope{ScopeType: types.APIKeyScopeTenant}, http.StatusForbidden},
		{"platform", &types.TenantAPIKeyScope{ScopeType: types.APIKeyScopePlatform}, http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			if test.scope != nil {
				engine.Use(func(c *gin.Context) {
					c.Request = c.Request.WithContext(
						types.WithTenantAPIKeyScope(c.Request.Context(), *test.scope),
					)
					c.Next()
				})
			}
			engine.PUT("/control-plane", RequirePlatformAPIKey(), func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/control-plane", nil))
			require.Equal(t, test.status, recorder.Code, recorder.Body.String())
		})
	}
}
