package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// GetTenantRetrievalConfig returns the tenant's global retrieval configuration.
func (h *TenantHandler) GetTenantRetrievalConfig(c *gin.Context) {
	ctx := c.Request.Context()
	tenant, _ := types.TenantInfoFromContext(ctx)
	if tenant == nil {
		logger.Error(ctx, "Workspace is empty")
		c.Error(errors.NewBadRequestError("Workspace is empty"))
		return
	}
	data := tenant.RetrievalConfig
	if data == nil {
		data = &types.RetrievalConfig{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// updateTenantRetrievalConfigInternal updates the tenant's global retrieval configuration.
func (h *TenantHandler) updateTenantRetrievalConfigInternal(c *gin.Context) {
	ctx := c.Request.Context()

	var cfg types.RetrievalConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}

	// Validate thresholds
	if cfg.VectorThreshold < 0 || cfg.VectorThreshold > 1 {
		c.Error(errors.NewBadRequestError("vector_threshold must be between 0 and 1"))
		return
	}
	if cfg.KeywordThreshold < 0 || cfg.KeywordThreshold > 1 {
		c.Error(errors.NewBadRequestError("keyword_threshold must be between 0 and 1"))
		return
	}
	if cfg.RerankThreshold < -10 || cfg.RerankThreshold > 10 {
		c.Error(errors.NewBadRequestError("rerank_threshold must be between -10 and 10"))
		return
	}
	if cfg.EmbeddingTopK < 0 || cfg.EmbeddingTopK > 200 {
		c.Error(errors.NewBadRequestError("embedding_top_k must be between 0 and 200"))
		return
	}
	if cfg.RerankTopK < 0 || cfg.RerankTopK > 200 {
		c.Error(errors.NewBadRequestError("rerank_top_k must be between 0 and 200"))
		return
	}

	tenant, _ := types.TenantInfoFromContext(ctx)
	if tenant == nil {
		logger.Error(ctx, "Workspace is empty")
		c.Error(errors.NewBadRequestError("Workspace is empty"))
		return
	}

	tenant.RetrievalConfig = &cfg
	updatedTenant, err := h.service.UpdateTenant(ctx, tenant)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update retrieval config").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updatedTenant.RetrievalConfig,
		"message": "Retrieval configuration updated successfully",
	})
}

// GetTenantMemoryConfig returns the workspace long-term memory configuration.
func (h *TenantHandler) GetTenantMemoryConfig(c *gin.Context) {
	ctx := c.Request.Context()
	tenant, _ := types.TenantInfoFromContext(ctx)
	if tenant == nil {
		logger.Error(ctx, "Workspace is empty")
		c.Error(errors.NewBadRequestError("Workspace is empty"))
		return
	}
	data := tenant.MemoryConfig
	if data == nil {
		// Memory is off until an admin turns it on: the feature retains what
		// users say across sessions, so it must not arrive enabled by default.
		data = &types.MemoryConfig{}
	}
	data.Normalize()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// updateTenantMemoryConfigInternal updates the workspace memory configuration.
func (h *TenantHandler) updateTenantMemoryConfigInternal(c *gin.Context) {
	ctx := c.Request.Context()

	var cfg types.MemoryConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	if cfg.WriteMode != "" &&
		cfg.WriteMode != types.MemoryWriteExplicitOnly &&
		cfg.WriteMode != types.MemoryWriteAuto {
		c.Error(errors.NewBadRequestError("write_mode must be explicit_only or auto"))
		return
	}
	if cfg.MaxItems < 0 || cfg.MaxItems > 2000 {
		c.Error(errors.NewBadRequestError("max_items must be between 0 and 2000"))
		return
	}
	if cfg.ExtractDelaySeconds < 0 || cfg.ExtractDelaySeconds > types.MaxMemoryExtractDelaySeconds {
		c.Error(errors.NewBadRequestError(fmt.Sprintf(
			"extract_delay_seconds must be between 0 and %d", types.MaxMemoryExtractDelaySeconds)))
		return
	}
	if cfg.ExtractMinIntervalSeconds < 0 ||
		cfg.ExtractMinIntervalSeconds > types.MaxMemoryExtractMinIntervalSeconds {
		c.Error(errors.NewBadRequestError(fmt.Sprintf(
			"extract_min_interval_seconds must be between 0 and %d",
			types.MaxMemoryExtractMinIntervalSeconds)))
		return
	}
	if len(cfg.EmbeddingModelID) > 64 {
		c.Error(errors.NewBadRequestError("embedding_model_id is too long"))
		return
	}
	if cfg.InterestThreshold < 0 || cfg.InterestThreshold > types.MaxMemoryInterestThreshold {
		c.Error(errors.NewBadRequestError(fmt.Sprintf(
			"interest_threshold must be between 1 and %d", types.MaxMemoryInterestThreshold)))
		return
	}
	if len([]rune(cfg.ExtractInstructions)) > types.MaxMemoryExtractInstructionsRunes {
		c.Error(errors.NewBadRequestError(fmt.Sprintf(
			"extract_instructions must be at most %d characters",
			types.MaxMemoryExtractInstructionsRunes)))
		return
	}
	cfg.Normalize()

	tenant, _ := types.TenantInfoFromContext(ctx)
	if tenant == nil {
		logger.Error(ctx, "Workspace is empty")
		c.Error(errors.NewBadRequestError("Workspace is empty"))
		return
	}

	tenant.MemoryConfig = &cfg
	updatedTenant, err := h.service.UpdateTenant(ctx, tenant)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to update memory config").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updatedTenant.MemoryConfig,
		"message": "Memory configuration updated successfully",
	})
}

func validateParserEngineOutboundURLs(cfg *types.ParserEngineConfig) error {
	if cfg == nil {
		return nil
	}
	if endpoint := strings.TrimSpace(cfg.MinerUEndpoint); endpoint != "" {
		if err := secutils.ValidateURLForSSRF(endpoint); err != nil {
			return fmt.Errorf("mineru_endpoint failed SSRF validation: %v", err)
		}
	}
	if vlmURL := strings.TrimSpace(cfg.MinerUVLMServerURL); vlmURL != "" {
		if err := secutils.ValidateURLForSSRF(vlmURL); err != nil {
			return fmt.Errorf("mineru_vlm_server_url failed SSRF validation: %v", err)
		}
	}
	if odlURL := strings.TrimSpace(cfg.ODLHybridURL); odlURL != "" {
		if err := secutils.ValidateURLForSSRF(odlURL); err != nil {
			return fmt.Errorf("odl_hybrid_url failed SSRF validation: %v", err)
		}
	}
	if endpoint := strings.TrimSpace(cfg.PaddleOCRVLEndpoint); endpoint != "" {
		if err := secutils.ValidateURLForSSRF(endpoint); err != nil {
			return fmt.Errorf("paddleocr_vl_endpoint failed SSRF validation: %v", err)
		}
	}
	return nil
}
