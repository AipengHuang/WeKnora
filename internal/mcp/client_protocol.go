package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
	protocol "github.com/mark3labs/mcp-go/mcp"
)

// Initialize 执行标准 MCP 初始化握手。
func (c *mcpGoClient) Initialize(ctx context.Context) (*InitializeResult, error) {
	if !c.connected {
		return nil, ErrNotConnected
	}
	req := protocol.InitializeRequest{Params: protocol.InitializeParams{
		ProtocolVersion: protocol.LATEST_PROTOCOL_VERSION,
		Capabilities:    protocol.ClientCapabilities{},
		ClientInfo: protocol.Implementation{
			Name: "WeKnora", Version: "1.0.0",
		},
	}}
	result, err := oauthCall(ctx, c, func() (*protocol.InitializeResult, error) {
		return c.client.Initialize(ctx, req)
	})
	if err != nil {
		c.checkErrorAndDisconnectIfNeeded(err)
		if oauthError := asOAuthRequired(err); oauthError != nil {
			return nil, oauthError
		}
		return nil, fmt.Errorf("failed to initialize: %w", err)
	}
	c.initialized = true
	return &InitializeResult{
		ProtocolVersion: result.ProtocolVersion,
		ServerInfo: ServerInfo{
			Name: result.ServerInfo.Name, Version: result.ServerInfo.Version,
			Title: result.ServerInfo.Title, Description: result.ServerInfo.Description,
		},
	}, nil
}

type protocolToolFields struct {
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema"`
	Annotations  json.RawMessage `json:"annotations"`
	Meta         json.RawMessage `json:"_meta"`
}

func convertProtocolTool(tool protocol.Tool) (*types.MCPTool, error) {
	definition, err := json.Marshal(tool)
	if err != nil {
		return nil, fmt.Errorf("failed to encode MCP tool definition %q: %w", tool.Name, err)
	}
	var fields protocolToolFields
	if err := json.Unmarshal(definition, &fields); err != nil {
		return nil, fmt.Errorf("failed to decode MCP tool definition %q: %w", tool.Name, err)
	}
	if len(fields.Meta) > 0 {
		if err := requireJSONObject(fields.Meta, "tool _meta"); err != nil {
			return nil, fmt.Errorf("invalid MCP tool definition %q: %w", tool.Name, err)
		}
	}
	return &types.MCPTool{
		Name: tool.Name, Description: tool.Description,
		InputSchema: fields.InputSchema, OutputSchema: fields.OutputSchema,
		Annotations: fields.Annotations, Meta: fields.Meta, Definition: definition,
	}, nil
}

// ListTools 保留 MCP 工具定义中的标准协议字段。
func (c *mcpGoClient) ListTools(ctx context.Context) ([]*types.MCPTool, error) {
	if !c.initialized {
		return nil, ErrNotConnected
	}
	result, err := oauthCall(ctx, c, func() (*protocol.ListToolsResult, error) {
		return c.client.ListTools(ctx, protocol.ListToolsRequest{})
	})
	if err != nil {
		c.checkErrorAndDisconnectIfNeeded(err)
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}
	tools := make([]*types.MCPTool, len(result.Tools))
	for index, tool := range result.Tools {
		converted, convertErr := convertProtocolTool(tool)
		if convertErr != nil {
			return nil, convertErr
		}
		tools[index] = converted
	}
	return tools, nil
}

// ListResources 获取 MCP 服务公开的资源。
func (c *mcpGoClient) ListResources(ctx context.Context) ([]*types.MCPResource, error) {
	if !c.initialized {
		return nil, ErrNotConnected
	}
	result, err := oauthCall(ctx, c, func() (*protocol.ListResourcesResult, error) {
		return c.client.ListResources(ctx, protocol.ListResourcesRequest{})
	})
	if err != nil {
		c.checkErrorAndDisconnectIfNeeded(err)
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}
	resources := make([]*types.MCPResource, len(result.Resources))
	for index, resource := range result.Resources {
		resources[index] = &types.MCPResource{
			URI: resource.URI, Name: resource.Name,
			Description: resource.Description, MimeType: resource.MIMEType,
		}
	}
	return resources, nil
}

type protocolCallResultFields struct {
	Content           json.RawMessage `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
	Meta              json.RawMessage `json:"_meta"`
}

func requireJSONObject(value json.RawMessage, field string) error {
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return fmt.Errorf("%s must be valid JSON: %w", field, err)
	}
	if _, ok := decoded.(map[string]any); !ok {
		return fmt.Errorf("%s must be a JSON object", field)
	}
	return nil
}

func requireJSONArray(value json.RawMessage, field string) error {
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return fmt.Errorf("%s must be valid JSON: %w", field, err)
	}
	if _, ok := decoded.([]any); !ok {
		return fmt.Errorf("%s must be a JSON array", field)
	}
	return nil
}

func convertProtocolCallResult(result *protocol.CallToolResult) (*CallToolResult, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to encode MCP tool result: %w", err)
	}
	var fields protocolCallResultFields
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, fmt.Errorf("failed to decode MCP tool result: %w", err)
	}
	if err := requireJSONArray(fields.Content, "content"); err != nil {
		return nil, err
	}
	if len(fields.StructuredContent) > 0 {
		if err := requireJSONObject(fields.StructuredContent, "structuredContent"); err != nil {
			return nil, err
		}
	}
	if len(fields.Meta) > 0 {
		if err := requireJSONObject(fields.Meta, "result _meta"); err != nil {
			return nil, err
		}
	}
	content := make([]ContentItem, 0, len(result.Content))
	for _, item := range result.Content {
		if textContent, ok := protocol.AsTextContent(item); ok {
			content = append(content, ContentItem{Type: "text", Text: textContent.Text})
		} else if imageContent, ok := protocol.AsImageContent(item); ok {
			content = append(content, ContentItem{
				Type: "image", Data: imageContent.Data, MimeType: imageContent.MIMEType,
			})
		}
	}
	return &CallToolResult{
		Content: content, RawContent: fields.Content, StructuredContent: fields.StructuredContent,
		Meta: fields.Meta, IsError: result.IsError,
	}, nil
}

// CallTool 调用 MCP 工具并保留标准结果元数据。
func (c *mcpGoClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	if !c.initialized {
		return nil, ErrNotConnected
	}
	req := protocol.CallToolRequest{Params: protocol.CallToolParams{Name: name, Arguments: args}}
	result, err := oauthCall(ctx, c, func() (*protocol.CallToolResult, error) {
		return c.client.CallTool(ctx, req)
	})
	if err != nil {
		c.checkErrorAndDisconnectIfNeeded(err)
		return nil, fmt.Errorf("failed to call tool: %w", err)
	}
	return convertProtocolCallResult(result)
}

// ReadResource 读取 MCP 资源内容。
func (c *mcpGoClient) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	if !c.initialized {
		return nil, ErrNotConnected
	}
	req := protocol.ReadResourceRequest{Params: protocol.ReadResourceParams{URI: uri}}
	result, err := oauthCall(ctx, c, func() (*protocol.ReadResourceResult, error) {
		return c.client.ReadResource(ctx, req)
	})
	if err != nil {
		c.checkErrorAndDisconnectIfNeeded(err)
		return nil, fmt.Errorf("failed to read resource: %w", err)
	}
	contents := make([]ResourceContent, 0, len(result.Contents))
	for _, item := range result.Contents {
		if textContent, ok := protocol.AsTextResourceContents(item); ok {
			contents = append(contents, ResourceContent{
				URI: textContent.URI, MimeType: textContent.MIMEType, Text: textContent.Text,
			})
		} else if blobContent, ok := protocol.AsBlobResourceContents(item); ok {
			contents = append(contents, ResourceContent{
				URI: blobContent.URI, MimeType: blobContent.MIMEType, Blob: blobContent.Blob,
			})
		}
	}
	return &ReadResourceResult{Contents: contents}, nil
}

func (c *mcpGoClient) IsConnected() bool {
	return c.connected
}

func (c *mcpGoClient) GetServiceID() string {
	return c.service.ID
}
