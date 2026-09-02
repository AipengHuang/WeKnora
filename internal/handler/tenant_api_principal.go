package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/form3tech-oss/jwt-go"
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

func apiPrincipalConfigForResponse(cfg *types.APIPrincipalConfig) apiPrincipalConfigResponse {
	if cfg == nil {
		cfg = &types.APIPrincipalConfig{}
	}
	return apiPrincipalConfigResponse{
		Mode:                  cfg.Mode,
		DirectHeaderName:      defaultAPIPrincipalDirectHeader,
		SignedTokenHeaderName: defaultAPIPrincipalTokenHeader,
		RequireDirectHeader:   cfg.RequireDirectHeader,
		HasHMACSecret:         strings.TrimSpace(cfg.HMACSecret) != "",
	}
}

// GetAPIPrincipalConfig godoc
// @Summary      获取空间 API Key 用户身份配置
// @Description  返回 X-API-Key 请求如何映射为终端 Principal 的配置（Owner）
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "空间ID"
// @Success      200  {object}  map[string]interface{}  "API principal 配置"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "权限不足"
// @Security     Bearer
// @Router       /tenants/{id}/api-principal-config [get]
func (h *TenantHandler) GetAPIPrincipalConfig(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(errors.NewBadRequestError("Invalid workspace ID"))
		return
	}
	tenant, err := h.service.GetTenantByID(ctx, id)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			c.Error(errors.NewInternalServerError("Failed to load workspace").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    apiPrincipalConfigForResponse(tenant.APIPrincipalConfig),
	})
}

// UpdateAPIPrincipalConfig godoc
// @Summary      更新空间 API Key 用户身份配置
// @Description  配置 X-API-Key 请求如何映射为终端 Principal（Owner）
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Param        id       path      int                           true  "空间ID"
// @Param        request  body      handler.apiPrincipalConfigRequest  true  "API principal 配置"
// @Success      200      {object}  map[string]interface{}        "更新后的配置"
// @Failure      400      {object}  errors.AppError               "请求参数错误"
// @Failure      403      {object}  errors.AppError               "权限不足"
// @Security     Bearer
// @Router       /tenants/{id}/api-principal-config [put]
func (h *TenantHandler) UpdateAPIPrincipalConfig(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(errors.NewBadRequestError("Invalid workspace ID"))
		return
	}
	var req apiPrincipalConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	switch req.Mode {
	case types.APIPrincipalModeTenant, types.APIPrincipalModeDirect, types.APIPrincipalModeSignedToken:
	default:
		c.Error(errors.NewValidationError("mode must be tenant, direct_header, or signed_token"))
		return
	}

	tenant, err := h.service.GetTenantByID(ctx, id)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			c.Error(errors.NewInternalServerError("Failed to load workspace").WithDetails(err.Error()))
		}
		return
	}

	existingSecret := ""
	if tenant.APIPrincipalConfig != nil {
		existingSecret = tenant.APIPrincipalConfig.HMACSecret
	}
	hmacSecret := existingSecret
	if req.HMACSecret != nil {
		hmacSecret = strings.TrimSpace(*req.HMACSecret)
	}
	cfg := &types.APIPrincipalConfig{
		Mode:                  req.Mode,
		DirectHeaderName:      defaultAPIPrincipalDirectHeader,
		SignedTokenHeaderName: defaultAPIPrincipalTokenHeader,
		RequireDirectHeader:   req.RequireDirectHeader,
		HMACSecret:            hmacSecret,
	}
	if cfg.Mode == types.APIPrincipalModeSignedToken && strings.TrimSpace(cfg.HMACSecret) == "" {
		c.Error(errors.NewValidationError("hmac_secret is required for signed_token mode"))
		return
	}
	tenant.APIPrincipalConfig = cfg

	updatedTenant, err := h.service.UpdateTenant(ctx, tenant)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			c.Error(errors.NewInternalServerError("Failed to update API principal config").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    apiPrincipalConfigForResponse(updatedTenant.APIPrincipalConfig),
	})
}

// CreateAPIPrincipalTestToken godoc
// @Summary      生成 API Playground 测试 JWT
// @Description  使用空间已保存的 HMAC 密钥签发短期外部用户 JWT（Owner）
// @Tags         空间管理
// @Accept       json
// @Produce      json
// @Param        id       path      int                                  true  "空间ID"
// @Param        request  body      handler.apiPrincipalTestTokenRequest true  "测试 Token 参数"
// @Success      200      {object}  map[string]interface{}               "短期 JWT"
// @Failure      400      {object}  errors.AppError                      "请求参数错误"
// @Failure      403      {object}  errors.AppError                      "权限不足"
// @Security     Bearer
// @Router       /tenants/{id}/api-principal-test-token [post]
func (h *TenantHandler) CreateAPIPrincipalTestToken(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(errors.NewBadRequestError("Invalid workspace ID"))
		return
	}

	var req apiPrincipalTestTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}

	externalUserID := strings.TrimSpace(req.ExternalUserID)
	if err := validateAPIPrincipalExternalUserID(externalUserID); err != nil {
		c.Error(errors.NewValidationError("external_user_id is invalid").WithDetails(err.Error()))
		return
	}

	tenant, err := h.service.GetTenantByID(ctx, id)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
		} else {
			c.Error(errors.NewInternalServerError("Failed to load workspace").WithDetails(err.Error()))
		}
		return
	}

	cfg := tenant.APIPrincipalConfig
	if cfg == nil || cfg.Mode != types.APIPrincipalModeSignedToken {
		c.Error(errors.NewValidationError("signed_token mode is required"))
		return
	}
	secret := strings.TrimSpace(cfg.HMACSecret)
	if secret == "" {
		c.Error(errors.NewValidationError("hmac_secret is required for signed_token mode"))
		return
	}

	ttl := defaultAPIPrincipalTestTokenTTL
	if req.ExpiresInSeconds > 0 {
		ttl = time.Duration(req.ExpiresInSeconds) * time.Second
	}
	if ttl <= 0 || ttl > maxAPIPrincipalTestTokenTTL {
		c.Error(errors.NewValidationError("expires_in_seconds must be between 1 and 3600"))
		return
	}

	now := time.Now()
	expiresAt := now.Add(ttl)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":       externalUserID,
		"tenant_id": strconv.FormatUint(id, 10),
		"aud":       "weknora",
		"iat":       now.Unix(),
		"exp":       expiresAt.Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		c.Error(errors.NewInternalServerError("Failed to create API principal test token").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": apiPrincipalTestTokenResponse{
			Token:            token,
			HeaderName:       defaultAPIPrincipalTokenHeader,
			ExpiresInSeconds: int(ttl.Seconds()),
			ExpiresAtUnix:    expiresAt.Unix(),
			ExternalUserID:   externalUserID,
		},
	})
}

func validateAPIPrincipalExternalUserID(id string) error {
	if id == "" {
		return errors.NewValidationError("external_user_id is required")
	}
	if len(id) > maxAPIPrincipalExternalUserIDLen {
		return errors.NewValidationError("external_user_id is too long")
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return errors.NewValidationError("external_user_id contains invalid characters")
		}
	}
	return nil
}
