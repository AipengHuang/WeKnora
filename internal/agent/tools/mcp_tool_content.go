package tools

import (
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/mcp"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	maxMCPImages    = 5
	maxMCPImageSize = 10 << 20
)

var allowedImageMIMEs = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

func mcpProtocolResultData(result *mcp.CallToolResult, tool *types.MCPTool) map[string]interface{} {
	data := map[string]interface{}{
		"content": result.RawContent,
		"isError": result.IsError,
	}
	if len(result.StructuredContent) > 0 {
		data["structuredContent"] = result.StructuredContent
	}
	if len(result.Meta) > 0 {
		data["_meta"] = result.Meta
	}
	if tool != nil && len(tool.Definition) > 0 {
		data["tool"] = tool.Definition
	}
	return data
}

func extractContentAndImages(content []mcp.ContentItem) (text string, images []string, skippedImages int) {
	var textParts []string
	for _, item := range content {
		switch item.Type {
		case "text":
			if item.Text != "" {
				textParts = append(textParts, item.Text)
			}
		case "image":
			mimeType := item.MimeType
			if mimeType == "" {
				mimeType = "image/png"
			}
			textParts = append(textParts, fmt.Sprintf("[Image: %s]", mimeType))
			if item.Data != "" && allowedImageMIMEs[mimeType] &&
				len(item.Data)*3/4 <= maxMCPImageSize && len(images) < maxMCPImages {
				images = append(images, fmt.Sprintf("data:%s;base64,%s", mimeType, item.Data))
			} else if item.Data != "" {
				skippedImages++
			}
		case "resource":
			textParts = append(textParts, fmt.Sprintf("[Resource: %s]", item.MimeType))
		default:
			if item.Text != "" {
				textParts = append(textParts, item.Text)
			} else if item.Data != "" {
				textParts = append(textParts, fmt.Sprintf("[Data: %s]", item.Type))
			}
		}
	}
	text = "Tool executed successfully (no text output)"
	if len(textParts) > 0 {
		text = strings.Join(textParts, "\n")
	}
	return text, images, skippedImages
}

func extractContentText(content []mcp.ContentItem) string {
	var textParts []string
	for _, item := range content {
		switch item.Type {
		case "text":
			if item.Text != "" {
				textParts = append(textParts, item.Text)
			}
		case "image":
			mimeType := item.MimeType
			if mimeType == "" {
				mimeType = "image"
			}
			textParts = append(textParts, fmt.Sprintf("[Image: %s]", mimeType))
		case "resource":
			textParts = append(textParts, fmt.Sprintf("[Resource: %s]", item.MimeType))
		default:
			if item.Text != "" {
				textParts = append(textParts, item.Text)
			} else if item.Data != "" {
				textParts = append(textParts, fmt.Sprintf("[Data: %s]", item.Type))
			}
		}
	}
	if len(textParts) == 0 {
		return "Tool executed successfully (no text output)"
	}
	return strings.Join(textParts, "\n")
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	var result strings.Builder
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' {
			result.WriteRune(char)
		}
	}
	return result.String()
}
