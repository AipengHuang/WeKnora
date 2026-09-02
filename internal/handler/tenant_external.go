package handler

import (
	"errors"
	"net/http"

	appErrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

type putExternalTenantRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=128"`
	Description string `json:"description" binding:"max=512"`
}

type externalTenantResponse struct {
	ID uint64 `json:"id"`
}

// PutExternalTenant 只供具备租户管理能力的平台密钥调用。
func (h *TenantHandler) PutExternalTenant(c *gin.Context) {
	ref, err := types.ParseExternalTenantRef(c.Param("external_ref"))
	if err != nil {
		c.Error(appErrors.NewValidationError(err.Error()))
		return
	}
	var request putExternalTenantRequest
	if !bindStrictControlPlaneJSON(c, &request) {
		return
	}
	stored, created, err := h.service.PutExternalTenant(c.Request.Context(), ref, &types.Tenant{
		Name:        request.Name,
		Description: request.Description,
	})
	if err != nil {
		if errors.Is(err, types.ErrExternalTenantDeleted) {
			c.Error(appErrors.NewConflictError("External tenant is deleted"))
			return
		}
		c.Error(appErrors.NewInternalServerError("Failed to provision workspace"))
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"success": true, "data": externalTenantResponse{ID: stored.ID}})
}
