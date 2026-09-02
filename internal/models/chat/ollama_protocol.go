package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Tencent/WeKnora/internal/types"
	ollamaapi "github.com/ollama/ollama/api"
)

const ollamaToolCallMetadataKey = "ollama"

type ollamaToolCallMetadata struct {
	FunctionIndex int `json:"function_index"`
}

// ollamaMessageText 保持思考内容与面向用户的答案严格分离。
func ollamaMessageText(message ollamaapi.Message) (string, string) {
	return message.Content, message.Thinking
}

// toolFrom 将本模块的工具定义转换为 Ollama 协议。
func (c *OllamaChat) toolFrom(tools []Tool) (ollamaapi.Tools, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	ollamaTools := make(ollamaapi.Tools, 0, len(tools))
	for _, tool := range tools {
		function := ollamaapi.ToolFunction{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
		}
		if len(tool.Function.Parameters) > 0 {
			if err := json.Unmarshal(tool.Function.Parameters, &function.Parameters); err != nil {
				return nil, fmt.Errorf("invalid Ollama tool schema for %q: %w", tool.Function.Name, err)
			}
		}
		ollamaTools = append(ollamaTools, ollamaapi.Tool{
			Type:     tool.Type,
			Function: function,
		})
	}
	return ollamaTools, nil
}

// toolCallFrom 使用持久化的供应商元数据还原 Ollama 函数索引。
func (c *OllamaChat) toolCallFrom(toolCalls []ToolCall) ([]ollamaapi.ToolCall, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}
	ollamaToolCalls := make([]ollamaapi.ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		if toolCall.ID == "" {
			return nil, errors.New("Ollama tool call ID is required")
		}
		index, err := ollamaFunctionIndex(toolCall.ProviderMetadata)
		if err != nil {
			return nil, err
		}
		args := ollamaapi.NewToolCallFunctionArguments()
		if toolCall.Function.Arguments != "" {
			if err := args.UnmarshalJSON([]byte(toolCall.Function.Arguments)); err != nil {
				return nil, fmt.Errorf("invalid Ollama tool call arguments: %w", err)
			}
		}
		ollamaToolCalls = append(ollamaToolCalls, ollamaapi.ToolCall{
			ID: toolCall.ID,
			Function: ollamaapi.ToolCallFunction{
				Index:     index,
				Name:      toolCall.Function.Name,
				Arguments: args,
			},
		})
	}
	return ollamaToolCalls, nil
}

// toolCallTo 保留 Ollama 返回的正式 ToolCall ID，缺失时直接拒绝。
func (c *OllamaChat) toolCallTo(ollamaToolCalls []ollamaapi.ToolCall) ([]types.LLMToolCall, error) {
	if len(ollamaToolCalls) == 0 {
		return nil, nil
	}
	toolCalls := make([]types.LLMToolCall, 0, len(ollamaToolCalls))
	for _, toolCall := range ollamaToolCalls {
		if toolCall.ID == "" {
			return nil, errors.New("Ollama tool call ID is required")
		}
		if toolCall.Function.Index < 0 {
			return nil, errors.New("Ollama tool call index must be non-negative")
		}
		argsBytes, err := json.Marshal(toolCall.Function.Arguments)
		if err != nil {
			return nil, fmt.Errorf("invalid Ollama tool call arguments: %w", err)
		}
		metadata, err := newOllamaToolCallMetadata(toolCall.Function.Index)
		if err != nil {
			return nil, err
		}
		toolCalls = append(toolCalls, types.LLMToolCall{
			ID:               toolCall.ID,
			Type:             "function",
			ProviderMetadata: metadata,
			Function: types.FunctionCall{
				Name:      toolCall.Function.Name,
				Arguments: string(argsBytes),
			},
		})
	}
	return toolCalls, nil
}

func newOllamaToolCallMetadata(index int) (types.ToolCallMetadata, error) {
	raw, err := json.Marshal(ollamaToolCallMetadata{FunctionIndex: index})
	if err != nil {
		return nil, fmt.Errorf("encode Ollama tool call metadata: %w", err)
	}
	return types.ToolCallMetadata{ollamaToolCallMetadataKey: raw}, nil
}

func ollamaFunctionIndex(metadata types.ToolCallMetadata) (int, error) {
	raw, exists := metadata[ollamaToolCallMetadataKey]
	if !exists {
		return 0, errors.New("Ollama tool call metadata is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var decoded ollamaToolCallMetadata
	if err := decoder.Decode(&decoded); err != nil {
		return 0, fmt.Errorf("invalid Ollama tool call metadata: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return 0, err
	}
	if decoded.FunctionIndex < 0 {
		return 0, errors.New("Ollama tool call index must be non-negative")
	}
	return decoded.FunctionIndex, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("Ollama tool call metadata must contain one JSON value")
		}
		return fmt.Errorf("invalid Ollama tool call metadata: %w", err)
	}
	return nil
}
