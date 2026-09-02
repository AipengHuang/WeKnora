package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// AgentStreamHandler handles agent events for SSE streaming
// It uses a dedicated EventBus per request to avoid SessionID filtering
// Events are appended to StreamManager without accumulation
type AgentStreamHandler struct {
	ctx                context.Context
	sessionID          string
	tenantID           uint64 // Tenant that owns this session; used when persisting skill artifacts.
	assistantMessageID string
	requestID          string
	receivedAt         time.Time // Handler entry timestamp, used for TTFB logging
	ttfbLogged         bool      // Guards one-shot TTFB log on first answer chunk
	assistantMessage   *types.Message
	streamManager      interfaces.StreamManager

	eventBus *event.EventBus

	// artifactCollector drains skill-generated files from the session
	// sandbox after the agent completes. Nil when the sandbox backend
	// doesn't support artifact collection or WeKnora was built without it.
	artifactCollector *service.ArtifactCollector

	// State tracking
	knowledgeRefs   []*types.SearchResult
	finalAnswer     string
	answerSegments  []*answerSegment     // Per-answer-event-ID accumulation, so superseded preambles can be dropped
	eventStartTimes map[string]time.Time // Track start time for duration calculation
	mu              sync.Mutex
}

// answerSegment accumulates the streamed content of a single final-answer event
// ID. A non-terminal round may stream a preamble ("let me search…") under its
// own answer ID and then be marked superseded once the round turns out to call
// tools; tracking segments separately lets us exclude that preamble from the
// persisted assistant message instead of leaking it into the final answer.
type answerSegment struct {
	id         string
	content    string
	superseded bool
}

// findAnswerSegment returns the segment for an answer event ID, or nil.
// Callers must hold h.mu.
func (h *AgentStreamHandler) findAnswerSegment(id string) *answerSegment {
	for _, seg := range h.answerSegments {
		if seg.id == id {
			return seg
		}
	}
	return nil
}

// composeFinalAnswer rebuilds the persisted answer from all non-superseded
// segments in arrival order. Callers must hold h.mu.
func (h *AgentStreamHandler) composeFinalAnswer() string {
	var b strings.Builder
	for _, seg := range h.answerSegments {
		if !seg.superseded {
			b.WriteString(seg.content)
		}
	}
	return b.String()
}

// NewAgentStreamHandler creates a new handler for agent SSE streaming
func NewAgentStreamHandler(
	ctx context.Context,
	sessionID, assistantMessageID, requestID string,
	tenantID uint64,
	receivedAt time.Time,
	assistantMessage *types.Message,
	streamManager interfaces.StreamManager,
	eventBus *event.EventBus,
	artifactCollector *service.ArtifactCollector,
) *AgentStreamHandler {
	return &AgentStreamHandler{
		ctx:                ctx,
		sessionID:          sessionID,
		tenantID:           tenantID,
		assistantMessageID: assistantMessageID,
		requestID:          requestID,
		receivedAt:         receivedAt,
		assistantMessage:   assistantMessage,
		streamManager:      streamManager,
		eventBus:           eventBus,
		artifactCollector:  artifactCollector,
		knowledgeRefs:      make([]*types.SearchResult, 0),
		eventStartTimes:    make(map[string]time.Time),
	}
}

// Subscribe subscribes to all agent streaming events on the dedicated EventBus
// No SessionID filtering needed since we have a dedicated EventBus per request
func (h *AgentStreamHandler) Subscribe() {
	// Subscribe to all agent streaming events on the dedicated EventBus
	h.eventBus.On(event.EventAgentThought, h.handleThought)
	h.eventBus.On(event.EventAgentToolCall, h.handleToolCall)
	h.eventBus.On(event.EventAgentToolResult, h.handleToolResult)
	h.eventBus.On(event.EventAgentReferences, h.handleReferences)
	h.eventBus.On(event.EventMemoryRecalled, h.handleMemoryRecalled)
	h.eventBus.On(event.EventAgentFinalAnswer, h.handleFinalAnswer)
	h.eventBus.On(event.EventAgentReflection, h.handleReflection)
	h.eventBus.On(event.EventError, h.handleError)
	h.eventBus.On(event.EventSessionTitle, h.handleSessionTitle)
	h.eventBus.On(event.EventAgentComplete, h.handleComplete)
	h.eventBus.On(event.EventToolApprovalRequired, h.handleToolApprovalRequired)
	h.eventBus.On(event.EventToolApprovalResolved, h.handleToolApprovalResolved)
	h.eventBus.On(event.EventMCPOAuthRequired, h.handleMCPOAuthRequired)
	h.eventBus.On(event.EventMCPOAuthResolved, h.handleMCPOAuthResolved)
}

// handleThought handles agent thought events
func (h *AgentStreamHandler) handleThought(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentThoughtData)
	if !ok {
		return nil
	}

	h.mu.Lock()

	// Track start time on first chunk
	if _, exists := h.eventStartTimes[evt.ID]; !exists {
		h.eventStartTimes[evt.ID] = time.Now()
	}

	// Calculate duration if done
	var metadata map[string]interface{}
	if data.Done {
		startTime := h.eventStartTimes[evt.ID]
		duration := time.Since(startTime)
		metadata = map[string]interface{}{
			"event_id":     evt.ID,
			"duration_ms":  duration.Milliseconds(),
			"completed_at": time.Now().Unix(),
		}
		delete(h.eventStartTimes, evt.ID)
	} else {
		metadata = map[string]interface{}{
			"event_id": evt.ID,
		}
	}

	h.mu.Unlock()

	// Append this chunk to stream (no accumulation - frontend will accumulate)
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeThinking,
		Content:   data.Content, // Just this chunk
		Done:      data.Done,
		Timestamp: time.Now(),
		Data:      metadata,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append thought event to stream failed", "error", err)
	}

	return nil
}

// handleToolCall handles tool call events
func (h *AgentStreamHandler) handleToolCall(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentToolCallData)
	if !ok {
		return nil
	}

	h.mu.Lock()
	// Track start time for this tool call (use tool_call_id as key)
	h.eventStartTimes[data.ToolCallID] = time.Now()
	// Any answer text streamed before this tool call was a non-terminal round's
	// preamble, not the final answer (the agent only ends by stopping naturally
	// with plain text and no tool calls). Drop those segments from the persisted
	// answer so the preamble never leaks into Message.Content.
	supersededAny := false
	for _, seg := range h.answerSegments {
		if !seg.superseded && seg.content != "" {
			seg.superseded = true
			supersededAny = true
		}
	}
	if supersededAny {
		h.finalAnswer = h.composeFinalAnswer()
	}
	h.mu.Unlock()

	metadata := map[string]interface{}{
		"tool_name":    data.ToolName,
		"arguments":    data.Arguments,
		"tool_call_id": data.ToolCallID,
	}

	// Append event to stream
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeToolCall,
		Content:   fmt.Sprintf("Calling tool: %s", data.ToolName),
		Done:      false,
		Timestamp: time.Now(),
		Data:      metadata,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append tool call event to stream failed", "error", err)
	}

	return nil
}

// handleToolResult handles tool result events
func (h *AgentStreamHandler) handleToolResult(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentToolResultData)
	if !ok {
		return nil
	}

	h.mu.Lock()
	// Calculate duration from start time if available, otherwise use provided duration
	var durationMs int64
	if startTime, exists := h.eventStartTimes[data.ToolCallID]; exists {
		durationMs = time.Since(startTime).Milliseconds()
		delete(h.eventStartTimes, data.ToolCallID)
	} else if data.Duration > 0 {
		// Fallback to provided duration if start time not tracked
		durationMs = data.Duration
	}
	h.mu.Unlock()

	// Send SSE response (both success and failure)
	responseType := types.ResponseTypeToolResult
	content := agenttools.StreamContentForToolResult(data.ToolName, data.Success, data.Error, data.Data)
	if !data.Success {
		responseType = types.ResponseTypeError
		if content == "" && data.Error != "" {
			content = data.Error
		}
	}

	// Build metadata including tool result data for rich frontend rendering
	metadata := map[string]interface{}{
		"tool_name":    data.ToolName,
		"success":      data.Success,
		"error":        data.Error,
		"duration_ms":  durationMs,
		"tool_call_id": data.ToolCallID,
	}

	clientData := agenttools.SanitizeToolResultForClient(data.ToolName, &types.ToolResult{
		Success: data.Success,
		Output:  data.Output,
		Error:   data.Error,
		Data:    data.Data,
	})
	for k, v := range clientData {
		metadata[k] = v
	}

	// Append event to stream
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      responseType,
		Content:   content,
		Done:      false,
		Timestamp: time.Now(),
		Data:      metadata,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append tool result event to stream failed", "error", err)
	}

	return nil
}

func toolApprovalDataToMap(v interface{}) map[string]interface{} {
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]interface{}{}
	}
	return m
}

// handleToolApprovalRequired persists MCP tool human-approval prompts for SSE / replay (issue #1173).
func (h *AgentStreamHandler) handleToolApprovalRequired(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.ToolApprovalRequiredData)
	if !ok {
		return nil
	}
	meta := toolApprovalDataToMap(data)
	meta["pending_id"] = data.PendingID
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeToolApprovalRequired,
		Content:   "MCP tool requires human approval",
		Done:      true,
		Timestamp: time.Now(),
		Data:      meta,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append tool approval required event failed", "error", err)
	}
	return nil
}

// handleToolApprovalResolved persists the outcome of a tool approval (issue #1173).
func (h *AgentStreamHandler) handleToolApprovalResolved(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.ToolApprovalResolvedData)
	if !ok {
		return nil
	}
	meta := toolApprovalDataToMap(data)
	meta["pending_id"] = data.PendingID
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeToolApprovalResolved,
		Content:   "MCP tool approval resolved",
		Done:      true,
		Timestamp: time.Now(),
		Data:      meta,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append tool approval resolved event failed", "error", err)
	}
	return nil
}

// handleMCPOAuthRequired forwards an in-conversation "authorize this MCP
// service" prompt to the SSE stream so the UI can render an Authorize card.
func (h *AgentStreamHandler) handleMCPOAuthRequired(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.MCPOAuthRequiredData)
	if !ok {
		return nil
	}
	meta := toolApprovalDataToMap(data)
	meta["pending_id"] = data.PendingID
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeMCPOAuthRequired,
		Content:   "MCP service requires OAuth authorization",
		Done:      true,
		Timestamp: time.Now(),
		Data:      meta,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append mcp oauth required event failed", "error", err)
	}
	return nil
}

// handleMCPOAuthResolved forwards the outcome of an in-conversation OAuth prompt.
func (h *AgentStreamHandler) handleMCPOAuthResolved(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.MCPOAuthResolvedData)
	if !ok {
		return nil
	}
	meta := toolApprovalDataToMap(data)
	meta["pending_id"] = data.PendingID
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeMCPOAuthResolved,
		Content:   "MCP OAuth authorization resolved",
		Done:      true,
		Timestamp: time.Now(),
		Data:      meta,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append mcp oauth resolved event failed", "error", err)
	}
	return nil
}
