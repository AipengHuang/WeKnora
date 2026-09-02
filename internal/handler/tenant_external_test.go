package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	appMiddleware "github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type externalTenantServiceStub struct {
	interfaces.TenantService
	created bool
}

func (s *externalTenantServiceStub) PutExternalTenant(
	_ context.Context,
	_ types.ExternalTenantRef,
	tenant *types.Tenant,
) (*types.Tenant, bool, error) {
	tenant.ID = 10001
	return tenant, s.created, nil
}

func externalTenantTestEngine(created bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(appMiddleware.ErrorHandler())
	handler := &TenantHandler{service: &externalTenantServiceStub{created: created}}
	engine.PUT("/external-tenants/:external_ref", handler.PutExternalTenant)
	return engine
}

func TestPutExternalTenantReturnsCreatedThenStableProjection(t *testing.T) {
	ref := "7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7"
	body := []byte(`{"name":"Workspace","description":"Knowledge"}`)
	for _, test := range []struct {
		created bool
		status  int
	}{{true, http.StatusCreated}, {false, http.StatusOK}} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/external-tenants/"+ref, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		externalTenantTestEngine(test.created).ServeHTTP(recorder, request)
		require.Equal(t, test.status, recorder.Code, recorder.Body.String())
		require.NotContains(t, recorder.Body.String(), "external_ref")
		require.Contains(t, recorder.Body.String(), `"id":10001`)
	}
}

func TestPutExternalTenantRejectsNonCanonicalProtocol(t *testing.T) {
	for _, test := range []struct {
		ref  string
		body string
	}{
		{"ACCOUNT-1", `{"name":"Workspace"}`},
		{"7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7", `{"name":"Workspace","legacy_id":"1"}`},
		{"7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7", `{"name":"Workspace"}{"name":"Other"}`},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/external-tenants/"+test.ref, bytes.NewBufferString(test.body))
		request.Header.Set("Content-Type", "application/json")
		externalTenantTestEngine(true).ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPut,
		"/external-tenants/7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7",
		bytes.NewBufferString(`{"name":"Workspace"}`),
	)
	externalTenantTestEngine(true).ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
}
