package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
	"github.com/Tencent/WeKnora/internal/types"
)

func (e *AgentEngine) runToolCall(
	ctx context.Context, tc types.LLMToolCall, i int,
	iteration, round int, sessionID, assistantMessageID string,
) types.ToolCall {
	tc.ID = agenttools.CanonicalToolCallID(agenttools.ToolCallCoordinates{
		ProviderID:         tc.ID,
		AssistantMessageID: assistantMessageID,
		Round:              round,
		Index:              i,
	})
	total := "?" // unknown in isolation; callers log the batch size
	toolTag := fmt.Sprintf("[Agent][Round-%d][Tool %s (%d/%s)]",
		round, tc.Function.Name, i+1, total)

	var args map[string]any
	argsStr := tc.Function.Arguments
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		repaired := agenttools.RepairJSON(argsStr)
		if repairErr := json.Unmarshal([]byte(repaired), &args); repairErr != nil {
			logger.Errorf(ctx, "%s Failed to parse arguments (repair failed): %v", toolTag, err)
			return types.ToolCall{
				ID:               tc.ID,
				Name:             tc.Function.Name,
				Args:             map[string]any{"_raw": argsStr},
				ProviderMetadata: tc.ProviderMetadata,
				Result: &types.ToolResult{
					Success: false,
					Error: fmt.Sprintf(
						"Failed to parse tool arguments: %v", err,
					) + "\n\nIf the JSON looks cut off, the previous round likely hit the output token cap. " +
						"Retry with complete JSON (required fields first) and a smaller payload.\n\n" +
						"[Analyze the error above and try a different approach.]",
				},
			}
		}
		logger.Warnf(ctx, "%s Repaired malformed JSON arguments", toolTag)
		// The initial model-context pass could not inspect malformed JSON.
		// Decode the repaired payload before execution, while preserving the
		// exact provider payload already stored in tc.ModelArguments.
		decoded := tc
		decoded.ModelArguments = ""
		decoded.Function.Arguments = repaired
		decodedCalls := []types.LLMToolCall{decoded}
		e.modelContext.DecodeToolCalls(decodedCalls)
		tc.Function.Arguments = decodedCalls[0].Function.Arguments
		tc.ArgumentResolution = decodedCalls[0].ArgumentResolution
		tc.UnresolvedHandles = decodedCalls[0].UnresolvedHandles
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return types.ToolCall{
				ID:               tc.ID,
				Name:             tc.Function.Name,
				Args:             map[string]any{"_raw": tc.Function.Arguments},
				ProviderMetadata: tc.ProviderMetadata,
				Result: &types.ToolResult{
					Success: false,
					Error:   fmt.Sprintf("Failed to parse repaired tool arguments: %v", err),
				},
			}
		}
	}

	logger.Debugf(ctx, "%s Args: %s", toolTag, tc.Function.Arguments)

	toolCallStartTime := time.Now()

	// Emit tool hint for UI progress display
	toolHint := formatToolHint(tc.Function.Name, args)
	e.eventBus.Emit(ctx, event.Event{
		ID:        tc.ID + "-tool-hint",
		Type:      event.EventAgentToolCall,
		SessionID: sessionID,
		Data: event.AgentToolCallData{
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Arguments:  args,
			Iteration:  iteration,
			Hint:       toolHint,
		},
	})

	common.PipelineInfo(ctx, "Agent", "tool_call_start", map[string]interface{}{
		"iteration":    iteration,
		"round":        round,
		"tool":         tc.Function.Name,
		"tool_call_id": tc.ID,
		"tool_index":   fmt.Sprintf("%d/%s", i+1, total),
	})

	// Open a Langfuse span for the tool invocation so the Langfuse UI shows
	// trace → agent.execute → agent.round.N → agent.tool.<name>, alongside
	// any nested generations (embedding/rerank/VLM) that the tool itself
	// triggers. No-op when Langfuse is disabled.
	mgr := langfuse.GetManager()
	// database_query's SQL is treated as sensitive by the UI hint layer
	// (toolHintSensitiveArgs) because it exposes implementation details.
	// Mirror that policy for Langfuse: redact raw arguments to avoid
	// leaking raw SQL into the observability backend.
	toolSpanInput := buildToolSpanInput(tc, args, toolHintSensitiveArgs[tc.Function.Name])
	argumentResolution, _ := toolSpanInput["argument_resolution"].(string)
	toolCtx, toolSpan := mgr.StartSpan(ctx, langfuse.SpanOptions{
		Name:  "agent.tool." + tc.Function.Name,
		Input: toolSpanInput,
		Metadata: map[string]interface{}{
			"iteration":               iteration,
			"round":                   round,
			"tool_index":              i + 1,
			"tool_call_id":            tc.ID,
			"session_id":              sessionID,
			"argument_resolution":     argumentResolution,
			"unresolved_handle_count": len(tc.UnresolvedHandles),
		},
	})

	principal, _ := types.PrincipalFromContext(ctx)
	execTimeout := toolExecutionTimeout(tc.Function.Name)
	toolExecCtx := agenttools.WithToolExecContext(toolCtx, &agenttools.ToolExecContext{
		SessionID:          sessionID,
		AssistantMessageID: assistantMessageID,
		EventBus:           e.eventBus,
		ToolCallID:         tc.ID,
		UserID:             principal.StorageID(),
		// ApprovalCtx keeps the round-level ctx without the per-tool execution timeout,
		// so MCP tool human-approval (issue #1173) can legitimately block longer.
		ApprovalCtx: toolCtx,
		ExecTimeout: execTimeout,
	})

	var result *types.ToolResult
	var err error
	if len(tc.UnresolvedHandles) > 0 {
		// A temporary handle is not an application identity. Fail before tool
		// execution so a hallucinated/stale cN/dN/bN/wN/iN/res:// token can
		// never reach persistence, an external service, or a routing decision.
		err = fmt.Errorf("tool arguments contain unresolved model handles: %v", tc.UnresolvedHandles)
	} else {
		execCtx, toolCancel := context.WithTimeout(toolExecCtx, execTimeout)
		result, err = e.toolRegistry.ExecuteTool(
			execCtx, tc.Function.Name,
			json.RawMessage(tc.Function.Arguments),
		)
		toolCancel()
	}
	duration := time.Since(toolCallStartTime).Milliseconds()

	toolCall := types.ToolCall{
		ID:               tc.ID,
		Name:             tc.Function.Name,
		Args:             args,
		Result:           result,
		Duration:         duration,
		ProviderMetadata: tc.ProviderMetadata,
	}

	if err != nil {
		logger.Errorf(ctx, "%s Failed in %dms: %v", toolTag, duration, err)
		toolCall.Result = &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}
	} else {
		success := result != nil && result.Success
		outputLen := 0
		if result != nil {
			outputLen = len(result.Output)
		}
		logger.Infof(ctx, "%s Completed in %dms: success=%v, output=%d chars",
			toolTag, duration, success, outputLen)
	}

	finishToolSpan(toolSpan, toolCall, err, duration)

	// Pipeline event for monitoring
	toolSuccess := toolCall.Result != nil && toolCall.Result.Success
	pipelineFields := map[string]interface{}{
		"iteration":    iteration,
		"round":        round,
		"tool":         tc.Function.Name,
		"tool_call_id": tc.ID,
		"duration_ms":  duration,
		"success":      toolSuccess,
	}
	if toolCall.Result != nil && toolCall.Result.Error != "" {
		pipelineFields["error"] = toolCall.Result.Error
	}
	if err != nil {
		common.PipelineError(ctx, "Agent", "tool_call_result", pipelineFields)
	} else if toolSuccess {
		common.PipelineInfo(ctx, "Agent", "tool_call_result", pipelineFields)
	} else {
		common.PipelineWarn(ctx, "Agent", "tool_call_result", pipelineFields)
	}

	if toolCall.Result != nil && toolCall.Result.Output != "" {
		preview := toolCall.Result.Output
		if len(preview) > 500 {
			preview = preview[:500] + "... (truncated)"
		}
		logger.Debugf(ctx, "%s Output preview:\n%s", toolTag, preview)
	}
	if toolCall.Result != nil && toolCall.Result.Error != "" {
		logger.Debugf(ctx, "%s Tool error: %s", toolTag, toolCall.Result.Error)
	}

	return toolCall
}
