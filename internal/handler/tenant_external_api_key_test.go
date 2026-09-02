package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appMiddleware "github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPutExternalTenantAPIKeyReturnsStableToken(t *testing.T) {
	tenantRef := "7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7"
	credentialRef := "a8af976f-47bd-5a9f-a270-4e92361e9a9d"
	body := []byte(`{"name":"adax-web-runtime-v1","capabilities":["chat","retrieve","manage_mcp_services"]}`)
	for _, test := range []struct {
		created bool
		status  int
	}{{true, http.StatusCreated}, {false, http.StatusOK}} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(
			http.MethodPut,
			"/external-tenants/"+tenantRef+"/api-keys/"+credentialRef,
			bytes.NewReader(body),
		)
		request.Header.Set("Content-Type", "application/json")
		externalTenantAPIKeyTestEngine(test.created).ServeHTTP(recorder, request)
		require.Equal(t, test.status, recorder.Code, recorder.Body.String())
		require.Contains(t, recorder.Body.String(), `"id":9001`)
		require.Contains(t, recorder.Body.String(), `"token":"stable-runtime-token"`)
		require.Contains(t, recorder.Body.String(), `"knowledge_base_ids":[]`)
		require.NotContains(t, recorder.Body.String(), `"api_key"`)
		require.NotContains(t, recorder.Body.String(), "external_ref")
	}
}

func TestPutExternalTenantAPIKeyRejectsNonCanonicalProtocol(t *testing.T) {
	tenantRef := "7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7"
	credentialRef := "a8af976f-47bd-5a9f-a270-4e92361e9a9d"
	for _, test := range []struct {
		path string
		body string
	}{
		{"/external-tenants/tenant-1/api-keys/" + credentialRef, `{"name":"runtime","capabilities":["chat"]}`},
		{"/external-tenants/" + tenantRef + "/api-keys/key-1", `{"name":"runtime","capabilities":["chat"]}`},
		{"/external-tenants/" + tenantRef + "/api-keys/" + credentialRef, `{"name":"runtime","capabilities":["chat"],"legacy_id":"1"}`},
		{"/external-tenants/" + tenantRef + "/api-keys/" + credentialRef, `{"name":"runtime","capabilities":["chat"]}{}`},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, test.path, bytes.NewBufferString(test.body))
		request.Header.Set("Content-Type", "application/json")
		externalTenantAPIKeyTestEngine(true).ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
}

func externalTenantAPIKeyTestEngine(created bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(appMiddleware.ErrorHandler())
	handler := &TenantHandler{
		apiKeyService: &externalTenantAPIKeyServiceStub{created: created},
	}
	engine.PUT(
		"/external-tenants/:external_ref/api-keys/:external_credential_ref",
		handler.PutExternalTenantAPIKey,
	)
	return engine
}

type externalTenantAPIKeyServiceStub struct {
	interfaces.TenantAPIKeyService
	created bool
}

func (s *externalTenantAPIKeyServiceStub) PutExternalTenantAPIKey(
	_ context.Context,
	req interfaces.ExternalTenantAPIKeyPutRequest,
) (*interfaces.ExternalTenantAPIKeyPutResult, error) {
	tenantID := uint64(10001)
	externalRef := req.CredentialRef.String()
	return &interfaces.ExternalTenantAPIKeyPutResult{
		APIKey: &types.TenantAPIKey{
			ID: 9001, TenantID: &tenantID, ExternalRef: &externalRef,
			ScopeType: types.APIKeyScopeTenant, Name: req.Name,
			KnowledgeBaseIDs: make(types.StringArray, 0),
			Capabilities:     types.StringArray{"chat", "retrieve", "manage_mcp_services"},
			CreatedAt:        time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		},
		Token: "stable-runtime-token", Created: s.created,
	}, nil
}
