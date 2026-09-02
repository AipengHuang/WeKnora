package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Tencent/WeKnora/internal/agent/approval"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/mcp"
	"github.com/Tencent/WeKnora/internal/types"
)

func RegisterMCPTools(
	ctx context.Context,
	registry *ToolRegistry,
	services []*types.MCPService,
	mcpManager *mcp.MCPManager,
	gate approval.MCPApproval,
	oauthSession *MCPOAuthSession,
) (int, error) {
	if len(services) == 0 {
		return 0, nil
	}
	listToolsTimeout := 30 * time.Second
	if ctx == nil || ctx == context.Background() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), listToolsTimeout)
		defer cancel()
	}
	registered := 0
	authWaitTimeoutSeconds := 0
	if oauthSession != nil {
		authWaitTimeoutSeconds = oauthSession.AuthWaitTimeoutSeconds
	}
	registrationOAuth := oauthSessionForRegistration(ctx, oauthSession, listToolsTimeout)
	for _, service := range services {
		if !service.Enabled {
			continue
		}
		toolCallID := "mcp-register-" + service.ID
		client, err := getOrCreateMCPClientWithOAuthRetry(
			ctx, mcpManager, service, gate, registrationOAuth, "", toolCallID,
		)
		if err != nil {
			logger.GetLogger(ctx).Errorf("Failed to create MCP client for service %s: %v", service.Name, err)
			continue
		}
		isStdio := service.TransportType == types.MCPTransportStdio
		if isStdio {
			defer func() {
				if disconnectErr := client.Disconnect(); disconnectErr != nil {
					logger.GetLogger(ctx).Warnf("Failed to disconnect stdio MCP client after listing tools: %v", disconnectErr)
				}
			}()
		}
		listContext, cancel := context.WithTimeout(ctx, listToolsTimeout)
		mcpTools, err := client.ListTools(listContext)
		cancel()
		if err != nil && !isStdio {
			logger.GetLogger(ctx).Warnf("Failed to list tools from MCP service %s (will retry with fresh connection): %v", service.Name, err)
			_ = client.Disconnect()
			client, err = getOrCreateMCPClientWithOAuthRetry(
				ctx, mcpManager, service, gate, registrationOAuth, "", toolCallID,
			)
			if err != nil {
				logger.GetLogger(ctx).Errorf("Failed to reconnect MCP client for service %s: %v", service.Name, err)
				continue
			}
			retryContext, retryCancel := context.WithTimeout(ctx, listToolsTimeout)
			mcpTools, err = client.ListTools(retryContext)
			retryCancel()
		}
		if err != nil {
			logger.GetLogger(ctx).Errorf("Failed to list tools from MCP service %s: %v", service.Name, err)
			continue
		}
		for _, mcpTool := range mcpTools {
			tool := NewMCPTool(service, mcpTool, mcpManager, gate, authWaitTimeoutSeconds)
			toolName := tool.Name()
			if existing, lookupErr := registry.GetTool(toolName); lookupErr == nil {
				if current, ok := existing.(*MCPTool); ok && current.service.ID != service.ID {
					logger.GetLogger(ctx).Warnf(
						"MCP tool name collision: %q from service %q conflicts with service %q — skipped (first-wins)",
						toolName, service.Name, current.service.Name,
					)
				}
			}
			registry.RegisterTool(tool)
			registered++
			logger.GetLogger(ctx).Infof("Registered MCP tool: %s from service: %s", toolName, service.Name)
		}
	}
	return registered, nil
}

func MCPToolNamesByServiceID(registry *ToolRegistry) map[string][]string {
	if registry == nil {
		return nil
	}
	result := make(map[string][]string)
	for _, name := range registry.ListTools() {
		tool, err := registry.GetTool(name)
		if err != nil {
			continue
		}
		mcpTool, ok := tool.(*MCPTool)
		if !ok || mcpTool.service == nil {
			continue
		}
		serviceID := mcpTool.service.ID
		result[serviceID] = append(result[serviceID], name)
	}
	for serviceID := range result {
		sort.Strings(result[serviceID])
	}
	return result
}

func GetMCPToolsInfo(
	ctx context.Context,
	services []*types.MCPService,
	mcpManager *mcp.MCPManager,
) (map[string][]string, error) {
	result := make(map[string][]string)
	infoContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for _, service := range services {
		if !service.Enabled {
			continue
		}
		client, err := mcpManager.GetOrCreateClient(ctx, service)
		if err != nil {
			continue
		}
		tools, err := client.ListTools(infoContext)
		if err != nil {
			continue
		}
		toolNames := make([]string, len(tools))
		for index, tool := range tools {
			toolNames[index] = tool.Name
		}
		result[service.Name] = toolNames
	}
	return result, nil
}

func SerializeMCPToolResult(result *types.ToolResult) (string, error) {
	if result == nil {
		return "", fmt.Errorf("result is nil")
	}
	if !result.Success {
		return fmt.Sprintf("Error: %s", result.Error), nil
	}
	output := result.Output
	if output == "" {
		output = "Success (no output)"
	}
	if result.Data != nil {
		if data, err := json.MarshalIndent(result.Data, "", "  "); err == nil {
			output += "\n\nStructured Data:\n" + string(data)
		}
	}
	return output, nil
}
