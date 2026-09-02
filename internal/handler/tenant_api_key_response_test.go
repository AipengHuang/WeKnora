package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// TestUpdateAPIKeyResponseHasNoUsableSecret 验证更新响应不包含可复用密钥。
func TestUpdateAPIKeyResponseHasNoUsableSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &TenantHandler{apiKeyService: &responseAPIKeyService{}}
	engine := gin.New()
	engine.PUT("/tenants/:id/api-keys/:key_id", handler.UpdateAPIKey)
	request := httptest.NewRequest(http.MethodPut, "/tenants/42/api-keys/7", strings.NewReader(
		`{"name":"runtime","full_access":true,"knowledge_base_ids":[],"capabilities":[]}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200 body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if payload.Data["api_key"] != "" {
		t.Fatal("tenant API key update response contains a usable secret")
	}
}

// TestCreateAPIKeyResponseReturnsOneTimeToken 验证只有创建响应允许返回一次性令牌。
func TestCreateAPIKeyResponseReturnsOneTimeToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	apiKeyService := &responseAPIKeyService{}
	handler := &TenantHandler{apiKeyService: apiKeyService}
	engine := gin.New()
	engine.POST("/tenants/:id/api-keys", handler.CreateAPIKey)
	request := httptest.NewRequest(http.MethodPost, "/tenants/42/api-keys", strings.NewReader(
		`{"name":"runtime","full_access":true,"knowledge_base_ids":[],"capabilities":[]}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201 body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if payload.Data["api_key"] != "one-time-test-key" || payload.Data["token"] != "one-time-test-key" {
		t.Fatal("create response did not return the one-time token")
	}
	if apiKeyService.createRequest.ScopeType != types.APIKeyScopeTenant {
		t.Fatalf("scope type = %q, want tenant", apiKeyService.createRequest.ScopeType)
	}
}

type responseAPIKeyService struct {
	interfaces.TenantAPIKeyService
	createRequest interfaces.TenantAPIKeyCreateRequest
}

func (s *responseAPIKeyService) CreateAPIKey(
	_ context.Context,
	request interfaces.TenantAPIKeyCreateRequest,
) (*interfaces.TenantAPIKeyCreateResult, error) {
	s.createRequest = request
	return &interfaces.TenantAPIKeyCreateResult{
		APIKey: &types.TenantAPIKey{ID: 7, Name: "runtime", APIKey: "one-time-test-key", FullAccess: true},
		Token:  "one-time-test-key",
	}, nil
}

func (*responseAPIKeyService) UpdateAPIKey(
	context.Context,
	interfaces.TenantAPIKeyUpdateRequest,
) (*types.TenantAPIKey, error) {
	return &types.TenantAPIKey{ID: 7, Name: "runtime", FullAccess: true}, nil
}
