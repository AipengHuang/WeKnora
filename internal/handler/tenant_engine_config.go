package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// GetTenantWebSearchConfig godoc
// @Summary      获取空间网络搜索配置
// @Description  获取空间的网络搜索配置
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "网络搜索配置"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /tenants/kv/web-search-config [get]
func (h *TenantHandler) GetTenantWebSearchConfig(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start getting tenant web search config")
	// Get tenant
	tenant, _ := types.TenantInfoFromContext(ctx)
	if tenant == nil {
		logger.Error(ctx, "Workspace is empty")
		c.Error(errors.NewBadRequestError("Workspace is empty"))
		return
	}

	logger.Infof(ctx, "Tenant web search config retrieved successfully, Tenant ID: %d", tenant.ID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.WebSearchConfigForResponse(tenant.WebSearchConfig, true),
	})
}

// GetTenantParserEngineConfig returns the tenant's parser engine config (MinerU endpoint, API key, etc.).
func (h *TenantHandler) GetTenantParserEngineConfig(c *gin.Context) {
	ctx := c.Request.Context()
	tenant, _ := types.TenantInfoFromContext(ctx)
	if tenant == nil {
		logger.Error(ctx, "Workspace is empty")
		c.Error(errors.NewBadRequestError("Workspace is empty"))
		return
	}
	data := types.ParserEngineConfigForResponse(tenant.ParserEngineConfig, true)
	if data == nil {
		data = &types.ParserEngineConfig{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// updateTenantParserEngineConfigInternal updates the tenant's parser engine config.
func (h *TenantHandler) updateTenantParserEngineConfigInternal(c *gin.Context) {
	ctx := c.Request.Context()
	var cfg types.ParserEngineConfig
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
	merged := types.MergeParserEngineConfigForUpdate(&cfg, tenant.ParserEngineConfig)
	if err := validateParserEngineOutboundURLs(merged); err != nil {
		c.Error(errors.NewValidationError(err.Error()))
		return
	}
	tenant.ParserEngineConfig = merged
	updatedTenant, err := h.service.UpdateTenant(ctx, tenant)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update workspace parser engine config").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.ParserEngineConfigForResponse(updatedTenant.ParserEngineConfig, true),
		"message": "解析引擎配置已更新",
	})
}

// GetTenantStorageEngineConfig returns the tenant's storage engine config (Local, MinIO, COS parameters).
func (h *TenantHandler) GetTenantStorageEngineConfig(c *gin.Context) {
	ctx := c.Request.Context()
	tenant, _ := types.TenantInfoFromContext(ctx)
	if tenant == nil {
		logger.Error(ctx, "Workspace is empty")
		c.Error(errors.NewBadRequestError("Workspace is empty"))
		return
	}
	data := types.StorageEngineConfigForResponse(tenant.StorageEngineConfig, true)
	if data == nil {
		data = &types.StorageEngineConfig{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// updateTenantStorageEngineConfigInternal updates the tenant's storage engine config.
func (h *TenantHandler) updateTenantStorageEngineConfigInternal(c *gin.Context) {
	ctx := c.Request.Context()
	var cfg types.StorageEngineConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.DefaultProvider))
	if provider == "" {
		provider = firstAllowedStorageProvider()
	}
	if provider == "" {
		c.Error(errors.NewBadRequestError("No storage provider is allowed by STORAGE_ALLOW_LIST"))
		return
	}
	if !isStorageProviderAllowed(provider) {
		c.Error(errors.NewBadRequestError("Storage provider is not allowed by STORAGE_ALLOW_LIST"))
		return
	}
	cfg.DefaultProvider = provider
	tenant, _ := types.TenantInfoFromContext(ctx)
	if tenant == nil {
		logger.Error(ctx, "Workspace is empty")
		c.Error(errors.NewBadRequestError("Workspace is empty"))
		return
	}
	merged := types.MergeStorageEngineConfigForUpdate(&cfg, tenant.StorageEngineConfig)
	tenant.StorageEngineConfig = merged
	updatedTenant, err := h.service.UpdateTenant(ctx, tenant)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update workspace storage engine config").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.StorageEngineConfigForResponse(updatedTenant.StorageEngineConfig, true),
		"message": "存储引擎配置已更新",
	})
}

// GetPromptTemplates godoc
// @Summary      获取提示词模板
// @Description  获取系统配置的提示词模板列表
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "提示词模板配置"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /tenants/kv/prompt-templates [get]
func (h *TenantHandler) GetPromptTemplates(c *gin.Context) {
	// Return prompt templates from config.yaml
	templates := h.config.PromptTemplates
	if templates == nil {
		templates = &config.PromptTemplatesConfig{}
	}

	// Determine user language from context (set by Language middleware)
	lang := types.LanguageFromContextOrDefault(c.Request.Context())

	// Build a localized copy so the original config is never mutated
	localized := &config.PromptTemplatesConfig{
		SystemPrompt:         config.LocalizeTemplates(templates.SystemPrompt, lang),
		ContextTemplate:      config.LocalizeTemplates(templates.ContextTemplate, lang),
		Rewrite:              config.LocalizeTemplates(templates.Rewrite, lang),
		Fallback:             config.LocalizeTemplates(templates.Fallback, lang),
		GenerateSessionTitle: templates.GenerateSessionTitle,
		GenerateSummary:      templates.GenerateSummary,
		KeywordsExtraction:   templates.KeywordsExtraction,
		AgentSystemPrompt:    config.LocalizeTemplates(templates.AgentSystemPrompt, lang),
		IntentPrompts:        config.LocalizeTemplates(templates.IntentPrompts, lang),
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    localized,
	})
}

// GetTenantChatHistoryConfig returns the tenant's chat history KB configuration.
func (h *TenantHandler) GetTenantChatHistoryConfig(c *gin.Context) {
	ctx := c.Request.Context()
	tenant, _ := types.TenantInfoFromContext(ctx)
	if tenant == nil {
		logger.Error(ctx, "Workspace is empty")
		c.Error(errors.NewBadRequestError("Workspace is empty"))
		return
	}
	data := tenant.ChatHistoryConfig
	if data == nil {
		data = &types.ChatHistoryConfig{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// updateTenantChatHistoryConfigInternal updates the tenant's chat history KB configuration.
// When enabled with an embedding model and no KB exists yet, it auto-creates a hidden KB.
func (h *TenantHandler) updateTenantChatHistoryConfigInternal(c *gin.Context) {
	ctx := c.Request.Context()

	// The frontend sends: enabled, embedding_model_id
	// knowledge_base_id is managed internally.
	var req types.ChatHistoryConfig
	if err := c.ShouldBindJSON(&req); err != nil {
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

	existing := tenant.ChatHistoryConfig

	// Build the new config, preserving the internally-managed knowledge_base_id
	cfg := &types.ChatHistoryConfig{
		Enabled:          req.Enabled,
		EmbeddingModelID: req.EmbeddingModelID,
		KnowledgeBaseID:  "", // will be set below
	}

	// Carry over existing KB ID if the embedding model hasn't changed
	if existing != nil && existing.KnowledgeBaseID != "" {
		if existing.EmbeddingModelID == req.EmbeddingModelID {
			cfg.KnowledgeBaseID = existing.KnowledgeBaseID
		} else {
			// Embedding model changed — the old KB is incompatible.
			// We'll create a new one below. The old KB remains but is orphaned (can be cleaned up later).
			logger.Infof(ctx, "Embedding model changed from %s to %s, will create new chat history KB", existing.EmbeddingModelID, req.EmbeddingModelID)
		}
	}

	// Auto-create hidden KB if enabled + model set + no KB yet
	if cfg.Enabled && cfg.EmbeddingModelID != "" && cfg.KnowledgeBaseID == "" {
		kb := &types.KnowledgeBase{
			Name:             "__chat_history__",
			Type:             types.KnowledgeBaseTypeDocument,
			IsTemporary:      true,
			Description:      "Auto-managed knowledge base for chat history message indexing",
			EmbeddingModelID: cfg.EmbeddingModelID,
		}
		createdKB, err := h.kbService.CreateKnowledgeBase(ctx, kb)
		if err != nil {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to create chat history knowledge base").WithDetails(err.Error()))
			return
		}
		cfg.KnowledgeBaseID = createdKB.ID
		logger.Infof(ctx, "Auto-created chat history KB: id=%s, embedding_model=%s", createdKB.ID, cfg.EmbeddingModelID)
	}

	tenant.ChatHistoryConfig = cfg
	updatedTenant, err := h.service.UpdateTenant(ctx, tenant)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update chat history config").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updatedTenant.ChatHistoryConfig,
		"message": "Chat history configuration updated successfully",
	})
}
