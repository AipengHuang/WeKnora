package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/handler/dto"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// DeleteTenant godoc
// @Summary      删除空间
// @Description  删除指定的空间
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "空间ID"
// @Success      200  {object}  map[string]interface{}  "删除成功"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /tenants/{id} [delete]
func (h *TenantHandler) DeleteTenant(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start deleting tenant")

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		logger.Errorf(ctx, "Invalid workspace ID: %s", secutils.SanitizeForLog(c.Param("id")))
		c.Error(errors.NewBadRequestError("Invalid workspace ID"))
		return
	}

	logger.Infof(ctx, "Deleting tenant, ID: %d", id)

	if err := h.service.DeleteTenant(ctx, id); err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to delete workspace: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to delete workspace").WithDetails(err.Error()))
		}
		return
	}

	logger.Infof(ctx, "Workspace deleted successfully, ID: %d", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Workspace deleted successfully",
	})
}

// ListTenants godoc
// @Summary      获取空间列表
// @Description  获取当前用户可访问的空间列表
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "空间列表"
// @Failure      500  {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Router       /tenants [get]
func (h *TenantHandler) ListTenants(c *gin.Context) {
	ctx := c.Request.Context()

	tenant, ok := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	if !ok || tenant == nil {
		c.Error(errors.NewUnauthorizedError("Authentication required"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": []*dto.TenantResponse{dto.NewTenantResponse(ctx, tenant)},
		},
	})
}

// ListAllTenants godoc
// @Summary      获取所有空间列表
// @Description  获取系统中所有空间（需要跨空间访问权限）
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "所有空间列表"
// @Failure      403  {object}  errors.AppError         "权限不足"
// @Security     Bearer
// @Router       /tenants/all [get]
func (h *TenantHandler) ListAllTenants(c *gin.Context) {
	ctx := c.Request.Context()

	// Cross-tenant gating (CanAccessAllTenants + EnableCrossTenantAccess)
	// is enforced at the route layer via middleware.RequireCrossTenantAccess
	// (router.go). The handler stays focused on listing.
	tenants, err := h.service.ListAllTenants(ctx)
	if err != nil {
		// Check if this is an application-specific error
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to retrieve all workspaces list: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to retrieve all workspaces list").WithDetails(err.Error()))
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": dto.NewTenantResponsesCrossTenant(tenants),
		},
	})
}

// SearchTenants godoc
// @Summary      搜索空间
// @Description  分页搜索空间（需要跨空间访问权限）
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Param        keyword    query     string  false  "搜索关键词"
// @Param        tenant_id  query     int     false  "空间ID筛选"
// @Param        page       query     int     false  "页码"  default(1)
// @Param        page_size  query     int     false  "每页数量"  default(20)
// @Success      200        {object}  map[string]interface{}  "搜索结果"
// @Failure      403        {object}  errors.AppError         "权限不足"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /tenants/search [get]
func (h *TenantHandler) SearchTenants(c *gin.Context) {
	ctx := c.Request.Context()

	// Cross-tenant gating is enforced at the route layer via
	// middleware.RequireCrossTenantAccess (router.go); the handler only
	// parses query params and delegates to the service.

	// Parse query parameters
	keyword := c.Query("keyword")
	tenantIDStr := c.Query("tenant_id")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	var tenantID uint64
	if tenantIDStr != "" {
		parsedID, err := strconv.ParseUint(tenantIDStr, 10, 64)
		if err == nil {
			tenantID = parsedID
		}
	}

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100 // Limit max page size
	}

	tenants, total, err := h.service.SearchTenants(ctx, keyword, tenantID, page, pageSize)
	if err != nil {
		// Check if this is an application-specific error
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to search workspaces: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to search workspaces").WithDetails(err.Error()))
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":     dto.NewTenantResponsesCrossTenant(tenants),
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetTenantKV godoc
// @Summary      获取空间KV配置
// @Description  获取空间级别的KV配置（支持web-search-config、prompt-templates、parser-engine-config、storage-engine-config、chat-history-config、retrieval-config）
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Param        key  path      string  true  "配置键名"
// @Success      200  {object}  map[string]interface{}  "配置值"
// @Failure      400  {object}  errors.AppError         "不支持的键"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /tenants/kv/{key} [get]
func (h *TenantHandler) GetTenantKV(c *gin.Context) {
	ctx := c.Request.Context()
	key := secutils.SanitizeForLog(c.Param("key"))

	switch key {
	case "web-search-config", "parser-engine-config", "storage-engine-config":
		if !dto.CanViewIntegrationSecrets(ctx) {
			c.Error(errors.NewForbiddenError("integration configuration requires admin access"))
			return
		}
	}

	switch key {
	case "web-search-config":
		h.GetTenantWebSearchConfig(c)
		return
	case "prompt-templates":
		h.GetPromptTemplates(c)
		return
	case "parser-engine-config":
		h.GetTenantParserEngineConfig(c)
		return
	case "storage-engine-config":
		h.GetTenantStorageEngineConfig(c)
		return
	case "chat-history-config":
		h.GetTenantChatHistoryConfig(c)
		return
	case "retrieval-config":
		h.GetTenantRetrievalConfig(c)
		return
	case "memory-config":
		h.GetTenantMemoryConfig(c)
		return
	default:
		logger.Info(ctx, "KV key not supported", "key", key)
		c.Error(errors.NewBadRequestError("unsupported key"))
		return
	}
}

// UpdateTenantKV godoc
// @Summary      更新空间KV配置
// @Description  更新空间级别的KV配置（支持web-search-config、parser-engine-config、storage-engine-config、chat-history-config、retrieval-config）
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Param        key      path      string  true  "配置键名"
// @Param        request  body      object  true  "配置值"
// @Success      200      {object}  map[string]interface{}  "更新成功"
// @Failure      400      {object}  errors.AppError         "不支持的键"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /tenants/kv/{key} [put]
func (h *TenantHandler) UpdateTenantKV(c *gin.Context) {
	ctx := c.Request.Context()
	key := secutils.SanitizeForLog(c.Param("key"))

	switch key {
	case "web-search-config", "parser-engine-config", "storage-engine-config":
		if !dto.CanViewIntegrationSecrets(ctx) {
			c.Error(errors.NewForbiddenError("integration configuration requires admin access"))
			return
		}
	}

	switch key {
	case "web-search-config":
		h.updateTenantWebSearchConfigInternal(c)
		return
	case "parser-engine-config":
		h.updateTenantParserEngineConfigInternal(c)
		return
	case "storage-engine-config":
		h.updateTenantStorageEngineConfigInternal(c)
		return
	case "chat-history-config":
		h.updateTenantChatHistoryConfigInternal(c)
		return
	case "retrieval-config":
		h.updateTenantRetrievalConfigInternal(c)
		return
	case "memory-config":
		h.updateTenantMemoryConfigInternal(c)
		return
	default:
		logger.Info(ctx, "KV key not supported", "key", key)
		c.Error(errors.NewBadRequestError("unsupported key"))
		return
	}
}

// updateTenantWebSearchConfigInternal updates tenant's web search config
func (h *TenantHandler) updateTenantWebSearchConfigInternal(c *gin.Context) {
	ctx := c.Request.Context()

	// Bind directly into the strong typed struct
	var cfg types.WebSearchConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}

	tenant, _ := types.TenantInfoFromContext(ctx)
	if tenant == nil {
		logger.Error(ctx, "Workspace is empty")
		c.Error(errors.NewBadRequestError("Workspace is empty"))
		return
	}

	cfg = *types.MergeWebSearchConfigForUpdate(&cfg, tenant.WebSearchConfig)

	// Validate configuration
	if cfg.MaxResults < 1 || cfg.MaxResults > 50 {
		c.Error(errors.NewBadRequestError("max_results must be between 1 and 50"))
		return
	}

	tenant.WebSearchConfig = &cfg
	updatedTenant, err := h.service.UpdateTenant(ctx, tenant)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			logger.Error(ctx, "Failed to update workspace: application error", appErr)
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update workspace web search config").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.WebSearchConfigForResponse(updatedTenant.WebSearchConfig, true),
		"message": "Web search configuration updated successfully",
	})
}
