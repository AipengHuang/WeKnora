package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/utils/ollama"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	ollamaapi "github.com/ollama/ollama/api"
)

// OllamaChat 实现了基于 Ollama 的聊天
type OllamaChat struct {
	modelName     string
	modelID       string
	ollamaService *ollama.OllamaService
}

// NewOllamaChat 创建 Ollama 聊天实例
func NewOllamaChat(config *ChatConfig, ollamaService *ollama.OllamaService) (*OllamaChat, error) {
	return &OllamaChat{
		modelName:     config.ModelName,
		modelID:       config.ModelID,
		ollamaService: ollamaService,
	}, nil
}

// convertMessages 转换消息格式为Ollama API格式
func (c *OllamaChat) convertMessages(messages []Message) ([]ollamaapi.Message, error) {
	ollamaMessages := make([]ollamaapi.Message, 0, len(messages))
	for _, msg := range messages {
		toolCalls, err := c.toolCallFrom(msg.ToolCalls)
		if err != nil {
			return nil, err
		}
		msgOllama := ollamaapi.Message{
			Role:      msg.Role,
			Content:   msg.Content,
			ToolCalls: toolCalls,
		}
		if msg.Role == "tool" {
			msgOllama.ToolName = msg.Name
			msgOllama.ToolCallID = msg.ToolCallID
		}
		if len(msg.Images) > 0 && msg.Role == "user" {
			for _, imgURL := range msg.Images {
				if imgData := resolveImageForOllama(imgURL); imgData != nil {
					msgOllama.Images = append(msgOllama.Images, imgData)
				}
			}
		}
		ollamaMessages = append(ollamaMessages, msgOllama)
	}
	return ollamaMessages, nil
}

// resolveImageForOllama resolves an image URL into raw bytes for Ollama.
// Handles local serving paths (/files/...), data URIs, and remote HTTP URLs.
func resolveImageForOllama(imageURL string) ollamaapi.ImageData {
	if data := resolveImageURLForOllama(imageURL); data != nil {
		return data
	}
	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		if err := secutils.ValidateURLForSSRF(imageURL); err != nil {
			return nil
		}
		client := secutils.NewSSRFSafeHTTPClient(secutils.SSRFSafeHTTPClientConfig{
			Timeout:      30 * time.Second,
			MaxRedirects: 5,
		})
		resp, err := client.Get(imageURL)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
		if err != nil {
			return nil
		}
		return data
	}
	return nil
}

// buildChatRequest 构建聊天请求参数
func (c *OllamaChat) buildChatRequest(
	messages []Message,
	opts *ChatOptions,
	isStream bool,
) (*ollamaapi.ChatRequest, error) {
	ollamaMessages, err := c.convertMessages(messages)
	if err != nil {
		return nil, err
	}

	// 设置流式标志
	streamFlag := isStream

	// 构建请求参数
	chatReq := &ollamaapi.ChatRequest{
		Model:    c.modelName,
		Messages: ollamaMessages,
		Stream:   &streamFlag,
		Options:  make(map[string]interface{}),
	}

	// 添加可选参数
	if opts != nil {
		chatReq.Options["temperature"] = opts.Temperature
		if opts.TopP > 0 {
			chatReq.Options["top_p"] = opts.TopP
		}
		if opts.MaxTokens > 0 {
			chatReq.Options["num_predict"] = opts.MaxTokens
		}
		if opts.Thinking != nil {
			chatReq.Think = &ollamaapi.ThinkValue{
				Value: *opts.Thinking,
			}
		}
		if len(opts.Format) > 0 {
			chatReq.Format = opts.Format
		}
		if len(opts.Tools) > 0 {
			tools, err := c.toolFrom(opts.Tools)
			if err != nil {
				return nil, err
			}
			chatReq.Tools = tools
		}
	}

	return chatReq, nil
}

// Chat 进行非流式聊天
func (c *OllamaChat) Chat(ctx context.Context, messages []Message, opts *ChatOptions) (*types.ChatResponse, error) {
	// 确保模型可用
	if err := c.ensureModelAvailable(ctx); err != nil {
		return nil, err
	}

	// 构建请求参数
	chatReq, err := c.buildChatRequest(messages, opts, false)
	if err != nil {
		return nil, err
	}
	// 记录请求日志
	logger.GetLogger(ctx).Infof("Sending chat request to model %s", c.modelName)

	var responseContent string
	var reasoningContent string
	var toolCalls []types.LLMToolCall
	var promptTokens, completionTokens int

	// 使用 Ollama 客户端发送请求
	err = c.ollamaService.Chat(ctx, chatReq, func(resp ollamaapi.ChatResponse) error {
		responseContent, reasoningContent = ollamaMessageText(resp.Message)
		converted, err := c.toolCallTo(resp.Message.ToolCalls)
		if err != nil {
			return err
		}
		toolCalls = converted

		// 获取token计数
		if resp.EvalCount > 0 {
			promptTokens = resp.PromptEvalCount
			completionTokens = resp.EvalCount - promptTokens
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("Ollama chat request failed: %w", err)
	}

	usage := types.TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
	usage.MarkPromptCacheUnsupported()
	logUsage(ctx, c.modelName, &usage)

	return &types.ChatResponse{
		Content:          responseContent,
		ReasoningContent: reasoningContent,
		ToolCalls:        toolCalls,
		Usage:            usage,
	}, nil
}

// ChatStream 进行流式聊天
func (c *OllamaChat) ChatStream(
	ctx context.Context,
	messages []Message,
	opts *ChatOptions,
) (<-chan types.StreamResponse, error) {
	// 确保模型可用
	if err := c.ensureModelAvailable(ctx); err != nil {
		return nil, err
	}

	// 构建请求参数
	chatReq, err := c.buildChatRequest(messages, opts, true)
	if err != nil {
		return nil, err
	}
	// 记录请求日志
	logger.GetLogger(ctx).Infof("Sending streaming chat request to model %s", c.modelName)

	// 创建流式响应通道
	streamChan := make(chan types.StreamResponse)

	// 启动goroutine处理流式响应
	go func() {
		defer close(streamChan)

		var thinking thinkingEmitter
		err := c.ollamaService.Chat(ctx, chatReq, func(resp ollamaapi.ChatResponse) error {
			// 发送思考内容（支持 Qwen3、DeepSeek 等推理模型）
			if resp.Message.Thinking != "" {
				thinking.emit(streamChan, resp.Message.Thinking)
			}

			if resp.Message.Content != "" {
				// 思考阶段结束后，发送思考完成事件
				thinking.finish(streamChan)
				streamChan <- types.StreamResponse{
					ResponseType: types.ResponseTypeAnswer,
					Content:      resp.Message.Content,
					Done:         false,
				}
			}

			if len(resp.Message.ToolCalls) > 0 {
				toolCalls, err := c.toolCallTo(resp.Message.ToolCalls)
				if err != nil {
					return err
				}
				streamChan <- types.StreamResponse{
					ResponseType: types.ResponseTypeToolCall,
					ToolCalls:    toolCalls,
					Done:         false,
				}

				// Ollama returns tool calls as complete objects (not incremental deltas).
				// Log this so we can trace non-streaming thought delivery.
				for _, tc := range resp.Message.ToolCalls {
					if tc.Function.Name == "thinking" {
						argsBytes, _ := json.Marshal(tc.Function.Arguments)
						logger.Warnf(ctx, "[Ollama Stream] Tool %q arrived non-incrementally (%d bytes args), "+
							"thought will not be token-streamed to frontend",
							tc.Function.Name, len(argsBytes))
					}
				}

				for _, tc := range resp.Message.ToolCalls {
					argsMap := tc.Function.Arguments.ToMap()
					switch tc.Function.Name {
					case "thinking":
						if thought, ok := argsMap["thought"].(string); ok && thought != "" {
							streamChan <- types.StreamResponse{
								ResponseType: types.ResponseTypeThinking,
								Content:      thought,
								Done:         false,
								Data: map[string]interface{}{
									"source":       "thinking_tool",
									"tool_call_id": tc.ID,
								},
							}
						}
					}
				}
			}

			if resp.Done {
				var usage *types.TokenUsage
				if resp.PromptEvalCount > 0 || resp.EvalCount > 0 {
					usage = &types.TokenUsage{
						PromptTokens:     resp.PromptEvalCount,
						CompletionTokens: resp.EvalCount,
						TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
					}
					usage.MarkPromptCacheUnsupported()
				}
				logUsage(ctx, c.modelName, usage)
				streamChan <- types.StreamResponse{
					ResponseType: types.ResponseTypeAnswer,
					Done:         true,
					Usage:        usage,
				}
			}

			return nil
		})
		if err != nil {
			logger.GetLogger(ctx).Errorf("Ollama streaming chat request failed: %v", err)
			// 发送错误响应
			streamChan <- types.StreamResponse{
				ResponseType: types.ResponseTypeError,
				Content:      err.Error(),
				Done:         true,
			}
		}
	}()

	return streamChan, nil
}

// 确保模型可用
func (c *OllamaChat) ensureModelAvailable(ctx context.Context) error {
	logger.GetLogger(ctx).Infof("Ensuring model %s is available", c.modelName)
	return c.ollamaService.EnsureModelAvailable(ctx, c.modelName)
}

// GetModelName 获取模型名称
func (c *OllamaChat) GetModelName() string {
	return c.modelName
}

// GetModelID 获取模型ID
func (c *OllamaChat) GetModelID() string {
	return c.modelID
}
