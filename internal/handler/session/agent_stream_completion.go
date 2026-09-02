package session

import (
	"context"
	"fmt"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// handleComplete handles agent complete events
func (h *AgentStreamHandler) handleComplete(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentCompleteData)
	if !ok {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Update assistant message with final data
	if data.MessageID == h.assistantMessageID {
		// h.assistantMessage.Content = data.FinalAnswer
		h.assistantMessage.IsCompleted = true
		h.assistantMessage.AgentDurationMs = data.TotalDurationMs

		// Update knowledge references if provided
		if len(data.KnowledgeRefs) > 0 {
			knowledgeRefs := make([]*types.SearchResult, 0, len(data.KnowledgeRefs))
			for _, ref := range data.KnowledgeRefs {
				if sr, ok := ref.(*types.SearchResult); ok {
					knowledgeRefs = append(knowledgeRefs, sr)
				}
			}
			h.assistantMessage.KnowledgeReferences = knowledgeRefs
		}

		h.assistantMessage.Content += data.FinalAnswer

		// Update agent steps if provided
		if data.AgentSteps != nil {
			if steps, ok := data.AgentSteps.([]types.AgentStep); ok {
				h.assistantMessage.AgentSteps = agenttools.SanitizeAgentStepsForStorage(steps)
			}
		}

		// Persist the turn's aggregated LLM usage with the message so history
		// reads still carry it after the live stream is gone.
		if usage, ok := data.Usage.(*types.TokenUsage); ok && usage != nil {
			h.assistantMessage.Usage = usage
		}

		// 仅从显式工具调用目录收集产物，避免并行工具互相串绑。
		if h.artifactCollector != nil {
			collectCtx := context.WithoutCancel(h.ctx)
			artifacts := make(types.MessageArtifacts, 0)
			for _, toolCallID := range artifactToolCallIDs(h.assistantMessage.AgentSteps) {
				collected, err := h.artifactCollector.CollectWithNotify(
					collectCtx,
					h.sessionID,
					h.assistantMessageID,
					h.tenantID,
					toolCallID,
					h.emitArtifactsPending,
				)
				if err != nil {
					logger.GetLogger(h.ctx).Warnf(
						"artifact collect failed session=%s message=%s tool_call=%s: %v",
						h.sessionID, h.assistantMessageID, toolCallID, err,
					)
					continue
				}
				artifacts = append(artifacts, collected...)
			}
			if len(artifacts) > 0 {
				h.assistantMessage.Artifacts = artifacts
				// The answer text names generated files the way the model saw
				// them in the sandbox. Bind those names to artifact indices now
				// that the index space is final, so a reloaded conversation
				// renders them instead of showing a broken link.
				h.assistantMessage.Content = rewriteArtifactReferences(
					h.assistantMessage.Content, artifacts,
				)
				logger.GetLogger(h.ctx).Infof(
					"artifact collect attached %d file(s) to message=%s session=%s",
					len(artifacts), h.assistantMessageID, h.sessionID,
				)
			}
		}
	}

	// Send completion event to stream manager so SSE can detect completion
	completeData := map[string]interface{}{
		"total_steps":       data.TotalSteps,
		"total_duration_ms": data.TotalDurationMs,
	}
	// Attach the freshly-collected artifacts so the frontend can render the
	// download button without waiting for a page refresh. We strip the
	// storage URL and any other server-only fields via publicArtifactViews
	// — clients only ever download through /artifacts/:index which enforces
	// tenant ownership.
	if len(h.assistantMessage.Artifacts) > 0 {
		completeData["artifacts"] = publicArtifactViews(h.assistantMessage.Artifacts)
	}
	// Carry the turn's aggregated LLM usage both inside data (map consumers)
	// and on the typed event field, which buildStreamResponse promotes to the
	// response's top-level usage.
	turnUsage, _ := data.Usage.(*types.TokenUsage)
	if turnUsage != nil {
		completeData["usage"] = turnUsage
	}
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeComplete,
		Content:   "",
		Done:      true,
		Timestamp: time.Now(),
		Data:      completeData,
		Usage:     turnUsage,
	}); err != nil {
		logger.GetLogger(h.ctx).Errorf("Append complete event to stream failed: %v", err)
	}

	return nil
}

// emitArtifactsPending tells the live UI that sandbox files exist and are
// being uploaded. It must not take h.mu — Collect calls it while
// handleComplete already holds the lock.
func (h *AgentStreamHandler) emitArtifactsPending(count int) {
	if h == nil || h.streamManager == nil || count <= 0 {
		return
	}
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        fmt.Sprintf("artifacts-pending-%d", time.Now().UnixMilli()),
		Type:      types.ResponseTypeArtifactsPending,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"count": count,
		},
	}); err != nil {
		logger.GetLogger(h.ctx).Warnf(
			"append artifacts_pending failed session=%s message=%s: %v",
			h.sessionID, h.assistantMessageID, err,
		)
	}
}

// publicArtifactViews returns a redacted view of the artifact list suitable
// for direct serialization onto the SSE stream. The physical storage path is
// stripped; the resource handle is kept because it is what the answer body
// references, and the frontend needs it to tie an inline reference to the file
// it names. Bytes are fetched through /artifacts/:index/download.
func publicArtifactViews(list types.MessageArtifacts) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(list))
	for i, a := range list {
		out = append(out, map[string]interface{}{
			"index":        i,
			"handle":       artifactHandle(a),
			"tool_call_id": a.ToolCallID,
			"file_name":    a.FileName,
			"file_type":    a.FileType,
			"file_size":    a.FileSize,
			"source_path":  a.SourcePath,
			"mod_time":     a.ModTime,
			"created_at":   a.CreatedAt,
		})
	}
	return out
}

// artifactToolCallIDs 按持久化步骤顺序返回稳定且去重的工具调用 ID。
func artifactToolCallIDs(steps types.AgentSteps) []string {
	seen := make(map[string]struct{})
	ids := make([]string, 0)
	for _, step := range steps {
		for _, call := range step.ToolCalls {
			if call.ID == "" {
				continue
			}
			if _, exists := seen[call.ID]; exists {
				continue
			}
			seen[call.ID] = struct{}{}
			ids = append(ids, call.ID)
		}
	}
	return ids
}
