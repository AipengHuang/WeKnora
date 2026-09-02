package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type setDefaultModelServiceStub struct {
	interfaces.ModelService
	model *types.Model
}

func (s *setDefaultModelServiceStub) SetDefaultModel(context.Context, string) (*types.Model, error) {
	return s.model, nil
}

func TestSetDefaultModelReturnsTypedModelResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelHandler(&setDefaultModelServiceStub{model: &types.Model{
		ID: "chat-a", TenantID: 7, Name: "qwen3:0.6b", Type: types.ModelTypeKnowledgeQA,
		Source: types.ModelSourceLocal, Status: types.ModelStatusActive, IsDefault: true,
	}})
	router := gin.New()
	router.PUT("/models/:id/default", h.SetDefaultModel)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/models/chat-a/default", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ID        string `json:"id"`
			IsDefault bool   `json:"is_default"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, "chat-a", response.Data.ID)
	require.True(t, response.Data.IsDefault)
}
