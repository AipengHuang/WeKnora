package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestAPIPrincipalConfigResponsePreservesUnconfiguredMode(t *testing.T) {
	for _, config := range []*types.APIPrincipalConfig{nil, {}} {
		if mode := apiPrincipalConfigForResponse(config).Mode; mode != "" {
			t.Fatalf("response mode = %q, want unconfigured", mode)
		}
	}
}

func TestUpdateAPIPrincipalConfigRejectsEmptyMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.PUT("/tenants/:id/api-principal-config", (&TenantHandler{}).UpdateAPIPrincipalConfig)

	request := httptest.NewRequest(
		http.MethodPut,
		"/tenants/42/api-principal-config",
		strings.NewReader(`{"hmac_secret":"test-secret"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", response.Code, response.Body.String())
	}
}
