package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/handler/dto"
	"github.com/Tencent/WeKnora/internal/logger"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

type setDefaultModelResponse struct {
	Success bool               `json:"success"`
	Data    *dto.ModelResponse `json:"data"`
}

// SetDefaultModel 将指定模型提升为同类型的唯一默认模型。
func (h *ModelHandler) SetDefaultModel(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(errors.NewBadRequestError("Model ID cannot be empty"))
		return
	}

	model, err := h.service.SetDefaultModel(ctx, id)
	if err != nil {
		if err == service.ErrModelNotFound {
			c.Error(errors.NewNotFoundError("Model not found"))
			return
		}
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"model_id": id})
		c.Error(errors.NewInternalServerError("failed to set default model"))
		return
	}

	c.JSON(http.StatusOK, setDefaultModelResponse{
		Success: true,
		Data:    dto.NewModelResponse(ctx, model),
	})
}
