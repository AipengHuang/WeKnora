package handler

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	appErrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

func bindStrictControlPlaneJSON(c *gin.Context, target any) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		c.Error(appErrors.NewValidationError("Content-Type must be application/json"))
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		c.Error(appErrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.Error(appErrors.NewValidationError("Request body must contain one JSON object"))
		return false
	}
	if err := binding.Validator.ValidateStruct(target); err != nil {
		c.Error(appErrors.NewValidationError("Invalid request parameters").WithDetails(err.Error()))
		return false
	}
	return true
}
