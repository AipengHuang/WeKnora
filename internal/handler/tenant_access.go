package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/handler/dto"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// GetTenant godoc
// @Summary      获取空间详情
// @Description  根据ID获取空间详情
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "空间ID"
// @Success      200  {object}  map[string]interface{}  "空间详情"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      404  {object}  errors.AppError         "空间不存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /tenants/{id} [get]
func (h *TenantHandler) GetTenant(c *gin.Context) {
	ctx := c.Request.Context()

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		logger.Errorf(ctx, "Invalid workspace ID: %s", secutils.SanitizeForLog(c.Param("id")))
		c.Error(errors.NewBadRequestError("Invalid workspace ID"))
		return
	}

	tenant, err := h.service.GetTenantByID(ctx, id)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to retrieve workspace: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to retrieve workspace").WithDetails(err.Error()))
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    dto.NewTenantResponse(ctx, tenant),
	})
}

// UpdateTenant godoc
// @Summary      更新空间
// @Description  更新空间信息
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Param        id       path      int           true  "空间ID"
// @Param        request  body      types.Tenant  true  "空间信息"
// @Success      200      {object}  map[string]interface{}  "更新后的空间"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /tenants/{id} [put]
func (h *TenantHandler) UpdateTenant(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start updating tenant")

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		logger.Errorf(ctx, "Invalid workspace ID: %s", secutils.SanitizeForLog(c.Param("id")))
		c.Error(errors.NewBadRequestError("Invalid workspace ID"))
		return
	}

	// Strict whitelist: only Name / Description are mutable through the
	// public PUT. Storage quota, status, business, configs, api_key and
	// every other privileged column live behind dedicated endpoints
	// (PUT /tenants/kv/:key, ...). Without this, an
	// Owner — including any user who just self-served a tenant — could
	// flip status / bump storage_quota by simply crafting an extended
	// JSON body. Pointers distinguish "field omitted" from "explicit
	// empty string" so we can leave untouched columns alone.
	var req updateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}

	// Load the persisted tenant so any column the request omits keeps
	// its current value through the GORM `Updates(struct)` zero-skip
	// behaviour (we always pass back the full struct).
	existing, err := h.service.GetTenantByID(ctx, id)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to load workspace").WithDetails(err.Error()))
		}
		return
	}

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			c.Error(errors.NewValidationError("name cannot be blank"))
			return
		}
		existing.Name = trimmed
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}

	logger.Infof(ctx, "Updating tenant, ID: %d, Name: %s", id, secutils.SanitizeForLog(existing.Name))

	updatedTenant, err := h.service.UpdateTenant(ctx, existing)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to update workspace: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update workspace").WithDetails(err.Error()))
		}
		return
	}

	logger.Infof(
		ctx,
		"Tenant updated successfully, ID: %d, Name: %s",
		updatedTenant.ID,
		secutils.SanitizeForLog(updatedTenant.Name),
	)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    dto.NewTenantResponse(ctx, updatedTenant),
	})
}

func (h *TenantHandler) ListAPIKeys(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(errors.NewBadRequestError("Invalid workspace ID"))
		return
	}
	keys, err := h.apiKeyService.ListAPIKeys(ctx, id)
	if err != nil {
		c.Error(errors.NewInternalServerError("Failed to list API keys").WithDetails(err.Error()))
		return
	}
	resp := make([]tenantAPIKeyResponse, 0, len(keys))
	for _, key := range keys {
		resp = append(resp, tenantAPIKeyForResponse(key))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

func (h *TenantHandler) CreateAPIKey(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(errors.NewBadRequestError("Invalid workspace ID"))
		return
	}
	var req tenantAPIKeyCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	if err := validateTenantAPIKeyRequest(ctx, h.kbService, id, req); err != nil {
		c.Error(err)
		return
	}
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := time.Unix(*req.ExpiresAt, 0).UTC()
		if !t.After(time.Now().UTC()) {
			c.Error(errors.NewValidationError("expires_at_unix must be in the future"))
			return
		}
		expiresAt = &t
	}
	result, err := h.apiKeyService.CreateAPIKey(ctx, interfaces.TenantAPIKeyCreateRequest{
		TenantID:         id,
		ScopeType:        types.APIKeyScopeTenant,
		Name:             req.Name,
		FullAccess:       req.FullAccess,
		KnowledgeBaseIDs: req.KnowledgeBaseIDs,
		Capabilities:     req.Capabilities,
		ExpiresAt:        expiresAt,
	})
	if err != nil {
		c.Error(errors.NewInternalServerError("Failed to create API key").WithDetails(err.Error()))
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": tenantAPIKeyCreateResponse{
			tenantAPIKeyResponse: tenantAPIKeyForResponse(result.APIKey),
			Token:                result.Token,
		},
	})
}

// UpdateAPIKey 修改已创建租户 API Key 的授权范围和其他可配置属性。
// 路由层要求当前租户 Owner；字段校验与创建接口保持一致。
func (h *TenantHandler) UpdateAPIKey(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || tenantID == 0 {
		c.Error(errors.NewBadRequestError("Invalid workspace ID"))
		return
	}
	keyID, err := strconv.ParseUint(c.Param("key_id"), 10, 64)
	if err != nil || keyID == 0 {
		c.Error(errors.NewBadRequestError("Invalid API key ID"))
		return
	}
	var req tenantAPIKeyUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	if appErr := validateTenantAPIKeyRequest(ctx, h.kbService, tenantID, tenantAPIKeyCreateRequest(req)); appErr != nil {
		c.Error(appErr)
		return
	}
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := time.Unix(*req.ExpiresAt, 0).UTC()
		expiresAt = &t
	}

	updated, err := h.apiKeyService.UpdateAPIKey(ctx, interfaces.TenantAPIKeyUpdateRequest{
		TenantID: tenantID, APIKeyID: keyID, Name: req.Name, FullAccess: req.FullAccess,
		KnowledgeBaseIDs: req.KnowledgeBaseIDs, Capabilities: req.Capabilities, ExpiresAt: expiresAt,
	})
	if err != nil {
		c.Error(errors.NewNotFoundError("API key not found"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": tenantAPIKeyForResponse(updated)})
}

func (h *TenantHandler) DeleteAPIKey(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(errors.NewBadRequestError("Invalid workspace ID"))
		return
	}
	keyID, err := strconv.ParseUint(c.Param("key_id"), 10, 64)
	if err != nil || keyID == 0 {
		c.Error(errors.NewBadRequestError("Invalid API key ID"))
		return
	}
	if err := h.apiKeyService.RevokeAPIKey(ctx, tenantID, keyID); err != nil {
		c.Error(errors.NewNotFoundError("API key not found"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func tenantAPIKeyForResponse(key *types.TenantAPIKey) tenantAPIKeyResponse {
	if key == nil {
		return tenantAPIKeyResponse{}
	}
	return tenantAPIKeyResponse{
		ID:               key.ID,
		ScopeType:        types.NormalizeAPIKeyScopeType(key.ScopeType),
		Name:             key.Name,
		APIKey:           key.APIKey,
		FullAccess:       key.FullAccess,
		KnowledgeBaseIDs: key.KnowledgeBaseIDs,
		Capabilities:     types.NormalizeAPIKeyCapabilities(key.Capabilities),
		LastUsedAt:       key.LastUsedAt,
		ExpiresAt:        key.ExpiresAt,
		CreatedAt:        key.CreatedAt,
	}
}

func validateTenantAPIKeyRequest(
	ctx context.Context,
	kbService interfaces.KnowledgeBaseService,
	tenantID uint64,
	req tenantAPIKeyCreateRequest,
) *errors.AppError {
	if strings.TrimSpace(req.Name) == "" {
		return errors.NewValidationError("name is required")
	}
	if req.FullAccess {
		return nil
	}
	caps := types.NormalizeAPIKeyCapabilities(types.StringArray(req.Capabilities))
	if len(caps) == 0 {
		return errors.NewValidationError("capabilities are required for scoped API keys")
	}
	for _, cap := range req.Capabilities {
		if strings.TrimSpace(cap) == "" {
			continue
		}
		if types.NormalizeAPIKeyCapability(types.APIKeyCapability(cap)) == "" {
			return errors.NewValidationError("capabilities contains an unknown capability")
		}
	}
	return validateTenantAPIKeyKnowledgeBaseIDs(ctx, kbService, tenantID, req.KnowledgeBaseIDs)
}

// validateTenantAPIKeyKnowledgeBaseIDs 校验白名单中的知识库真实存在且属于目标租户。
// 入参是请求上下文、知识库服务、租户 ID 和待授权 ID；成功无返回值，失败返回可直接响应的应用错误。
func validateTenantAPIKeyKnowledgeBaseIDs(
	ctx context.Context,
	kbService interfaces.KnowledgeBaseService,
	tenantID uint64,
	knowledgeBaseIDs []string,
) *errors.AppError {
	if len(knowledgeBaseIDs) == 0 {
		return nil
	}
	return validateTenantAPIKeyKnowledgeBaseIDsWithLookup(
		ctx, tenantID, knowledgeBaseIDs, kbService.GetKnowledgeBaseByID,
	)
}

// validateTenantAPIKeyKnowledgeBaseIDsWithLookup 将归属校验与大型知识库服务接口解耦，便于覆盖边界测试。
// lookup 输入知识库 ID 并返回真实知识库；函数输出 nil 或可直接响应的校验错误。
func validateTenantAPIKeyKnowledgeBaseIDsWithLookup(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseIDs []string,
	lookup func(context.Context, string) (*types.KnowledgeBase, error),
) *errors.AppError {
	for _, kbID := range knowledgeBaseIDs {
		kbID = strings.TrimSpace(kbID)
		if kbID == "" {
			continue
		}
		kb, err := lookup(ctx, kbID)
		if err != nil || kb == nil {
			return errors.NewValidationError("knowledge_base_ids contains an unknown knowledge base")
		}
		if kb.TenantID != tenantID {
			return errors.NewForbiddenError("knowledge_base_ids contains a knowledge base outside this workspace")
		}
	}
	return nil
}
