package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	appRepository "github.com/Tencent/WeKnora/internal/application/repository"
	appService "github.com/Tencent/WeKnora/internal/application/service"
	appMiddleware "github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPutExternalTenantAPIKeyConcurrentHTTPReplay(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "0123456789abcdef0123456789abcdef")
	db, err := gorm.Open(sqlite.Open(
		filepath.Join(t.TempDir(), "external-api-key-http.db")+"?_busy_timeout=5000&_journal_mode=WAL",
	), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.TenantAPIKey{}))
	tenantRef := "7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7"
	require.NoError(t, db.Create(&types.Tenant{
		ExternalRef: &tenantRef,
		Name:        "Workspace",
	}).Error)

	apiKeyRepository := appRepository.NewTenantAPIKeyRepository(db)
	apiKeyService := appService.NewTenantAPIKeyService(apiKeyRepository)
	engine := gin.New()
	engine.Use(appMiddleware.ErrorHandler())
	engine.PUT(
		"/external-tenants/:external_ref/api-keys/:external_credential_ref",
		(&TenantHandler{apiKeyService: apiKeyService}).PutExternalTenantAPIKey,
	)

	const callers = 8
	results := make(chan externalAPIKeyHTTPResult, callers)
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results <- requestExternalTenantAPIKey(engine, tenantRef)
		}()
	}
	wait.Wait()
	close(results)

	var first externalTenantAPIKeyResponse
	createdResponses := 0
	for result := range results {
		require.NoError(t, result.err)
		require.Contains(t, []int{http.StatusOK, http.StatusCreated}, result.status)
		if result.status == http.StatusCreated {
			createdResponses++
		}
		if first.ID == 0 {
			first = result.response
		}
		require.Equal(t, first.ID, result.response.ID)
		require.Equal(t, first.Token, result.response.Token)
		require.NotNil(t, result.response.KnowledgeBaseIDs)
		require.Empty(t, result.response.KnowledgeBaseIDs)
	}
	require.Equal(t, 1, createdResponses)
	require.NotEmpty(t, first.Token)

	var count int64
	require.NoError(t, db.Model(&types.TenantAPIKey{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	var encrypted string
	require.NoError(t, db.Raw(
		"SELECT api_key FROM tenant_api_keys WHERE id = ?", first.ID,
	).Scan(&encrypted).Error)
	require.NotEqual(t, first.Token, encrypted)
}

type externalAPIKeyHTTPResult struct {
	status   int
	response externalTenantAPIKeyResponse
	err      error
}

func requestExternalTenantAPIKey(
	engine http.Handler,
	tenantRef string,
) externalAPIKeyHTTPResult {
	credentialRef := "a8af976f-47bd-5a9f-a270-4e92361e9a9d"
	body := []byte(`{"name":"adax-web-runtime-v1","capabilities":["chat","retrieve","manage_mcp_services"]}`)
	request := httptest.NewRequest(
		http.MethodPut,
		"/external-tenants/"+tenantRef+"/api-keys/"+credentialRef,
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	var envelope struct {
		Success bool                         `json:"success"`
		Data    externalTenantAPIKeyResponse `json:"data"`
	}
	err := json.Unmarshal(recorder.Body.Bytes(), &envelope)
	return externalAPIKeyHTTPResult{
		status: recorder.Code, response: envelope.Data, err: err,
	}
}
