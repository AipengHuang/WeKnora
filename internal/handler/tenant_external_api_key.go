package handler

import (
	"errors"
	"net/http"
	"time"

	appErrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type putExternalTenantAPIKeyRequest struct {
	Name         string                   `json:"name" binding:"required,min=1,max=128"`
	Capabilities []types.APIKeyCapability `json:"capabilities" binding:"required,min=1,max=8,dive,required"`
}

type externalTenantAPIKeyResponse struct {
	ID               uint64                `json:"id"`
	ScopeType        types.APIKeyScopeType `json:"scope_type"`
	Name             string                `json:"name"`
	FullAccess       bool                  `json:"full_access"`
	KnowledgeBaseIDs types.StringArray     `json:"knowledge_base_ids"`
	Capabilities     types.StringArray     `json:"capabilities"`
	CreatedAt        time.Time             `json:"created_at"`
	Token            string                `json:"token"`
}

// PutExternalTenantAPIKey 只供具备租户管理能力的平台密钥调用。
func (h *TenantHandler) PutExternalTenantAPIKey(c *gin.Context) {
	tenantRef, err := types.ParseExternalTenantRef(c.Param("external_ref"))
	if err != nil {
		c.Error(appErrors.NewValidationError(err.Error()))
		return
	}
	credentialRef, err := types.ParseExternalTenantCredentialRef(
		c.Param("external_credential_ref"),
	)
	if err != nil {
		c.Error(appErrors.NewValidationError(err.Error()))
		return
	}
	var request putExternalTenantAPIKeyRequest
	if !bindStrictControlPlaneJSON(c, &request) {
		return
	}
	result, err := h.apiKeyService.PutExternalTenantAPIKey(
		c.Request.Context(),
		interfaces.ExternalTenantAPIKeyPutRequest{
			TenantRef: tenantRef, CredentialRef: credentialRef,
			Name: request.Name, Capabilities: request.Capabilities,
		},
	)
	if err != nil {
		switch {
		case errors.Is(err, types.ErrExternalTenantCredentialProtocol):
			c.Error(appErrors.NewValidationError(err.Error()))
		case errors.Is(err, types.ErrExternalTenantNotFound):
			c.Error(appErrors.NewNotFoundError("External tenant not found"))
		case errors.Is(err, types.ErrExternalTenantDeleted),
			errors.Is(err, types.ErrExternalTenantCredentialConflict):
			c.Error(appErrors.NewConflictError(err.Error()))
		default:
			c.Error(appErrors.NewInternalServerError("Failed to provision runtime credential"))
		}
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	key := result.APIKey
	c.JSON(status, gin.H{"success": true, "data": externalTenantAPIKeyResponse{
		ID: key.ID, ScopeType: key.ScopeType, Name: key.Name,
		FullAccess: key.FullAccess, KnowledgeBaseIDs: key.KnowledgeBaseIDs,
		Capabilities: key.Capabilities, CreatedAt: key.CreatedAt, Token: result.Token,
	}})
}
