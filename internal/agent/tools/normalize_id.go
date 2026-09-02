package tools

import (
	"crypto/sha256"
	"fmt"
)

// ToolCallCoordinates 是一次工具调用在 assistant 消息内的稳定坐标。
type ToolCallCoordinates struct {
	ProviderID         string
	AssistantMessageID string
	Round              int
	Index              int
}

// CanonicalToolCallID 保留供应商正式 ID；缺失时按稳定坐标生成协议 ID。
func CanonicalToolCallID(coordinates ToolCallCoordinates) string {
	if coordinates.ProviderID != "" {
		return coordinates.ProviderID
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%d\x00%d",
		coordinates.AssistantMessageID,
		coordinates.Round,
		coordinates.Index,
	)))
	return fmt.Sprintf("call_%x", hash[:12])
}
