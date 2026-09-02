package session

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// handleReferences handles knowledge references events
func (h *AgentStreamHandler) handleReferences(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentReferencesData)
	if !ok {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Extract knowledge references
	// Try to cast directly to []*types.SearchResult first
	if searchResults, ok := data.References.([]*types.SearchResult); ok {
		h.knowledgeRefs = append(h.knowledgeRefs, searchResults...)
	} else if refs, ok := data.References.([]interface{}); ok {
		// Fallback: convert from []interface{}
		for _, ref := range refs {
			if sr, ok := ref.(*types.SearchResult); ok {
				h.knowledgeRefs = append(h.knowledgeRefs, sr)
			} else if refMap, ok := ref.(map[string]interface{}); ok {
				// Parse from map if needed
				h.knowledgeRefs = append(h.knowledgeRefs, searchResultFromMap(refMap))
			}
		}
	}

	// Update assistant message references
	h.assistantMessage.KnowledgeReferences = h.knowledgeRefs

	// Append references event to stream
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeReferences,
		Content:   "",
		Done:      false,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"references": types.References(h.knowledgeRefs),
		},
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append references event to stream failed", "error", err)
	}

	return nil
}

// handleMemoryRecalled records the long-term memories injected into this turn.
// The list is both persisted on the assistant message and streamed, so the
// panel is present live and after a reload.
func (h *AgentStreamHandler) handleMemoryRecalled(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.MemoryRecalledData)
	if !ok {
		return nil
	}
	used, ok := data.Memories.(types.UsedMemories)
	if !ok || len(used) == 0 {
		return nil
	}

	h.mu.Lock()
	h.assistantMessage.UsedMemories = used
	h.mu.Unlock()

	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeMemoryRecalled,
		Done:      false,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"memories": used},
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append memory recalled event to stream failed", "error", err)
	}
	return nil
}

// handleFinalAnswer handles final answer events
func (h *AgentStreamHandler) handleFinalAnswer(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentFinalAnswerData)
	if !ok {
		return nil
	}

	h.mu.Lock()

	// Track start time on first chunk
	if _, exists := h.eventStartTimes[evt.ID]; !exists {
		h.eventStartTimes[evt.ID] = time.Now()
	}

	// Emit a one-shot TTFB log the first time *any* answer chunk reaches
	// the stream handler. This lets us compare the backend's "request in →
	// first token out" timing against the frontend-observed TTFB and pin
	// down where latency lives (network vs server vs LLM).
	if !h.ttfbLogged && !h.receivedAt.IsZero() {
		h.ttfbLogged = true
		ttfb := time.Since(h.receivedAt)
		logger.GetLogger(h.ctx).Infof("TTFB:first_answer_chunk request_id=%s, session_id=%s, ttfb_ms=%d",
			h.requestID, h.sessionID, ttfb.Milliseconds())
	}

	// Accumulate final answer locally for assistant message (database). Track
	// per event ID so a later supersede can subtract this segment's content.
	if data.Content != "" {
		seg := h.findAnswerSegment(evt.ID)
		if seg == nil {
			seg = &answerSegment{id: evt.ID}
			h.answerSegments = append(h.answerSegments, seg)
		}
		seg.content += data.Content
		h.finalAnswer = h.composeFinalAnswer()
	}
	if data.IsFallback {
		h.assistantMessage.IsFallback = true
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
	if data.IsFallback {
		metadata["is_fallback"] = true
	}
	h.mu.Unlock()

	// Append this chunk to stream (frontend will accumulate by event ID)
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeAnswer,
		Content:   data.Content, // Just this chunk
		Done:      data.Done,
		Timestamp: time.Now(),
		Data:      metadata,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append answer event to stream failed", "error", err)
	}

	return nil
}

// handleReflection handles agent reflection events
func (h *AgentStreamHandler) handleReflection(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.AgentReflectionData)
	if !ok {
		return nil
	}

	// Append this chunk to stream (frontend will accumulate by event ID)
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeReflection,
		Content:   data.Content, // Just this chunk
		Done:      data.Done,
		Timestamp: time.Now(),
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append reflection event to stream failed", "error", err)
	}

	return nil
}

// handleError handles error events
func (h *AgentStreamHandler) handleError(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.ErrorData)
	if !ok {
		return nil
	}

	// Build error metadata
	metadata := map[string]interface{}{
		"stage": data.Stage,
		"error": data.Error,
	}

	// Append error event to stream
	if err := h.streamManager.AppendEvent(h.ctx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeError,
		Content:   data.Error,
		Done:      true,
		Timestamp: time.Now(),
		Data:      metadata,
	}); err != nil {
		logger.GetLogger(h.ctx).Error("Append error event to stream failed", "error", err)
	}

	return nil
}

// handleSessionTitle handles session title update events
func (h *AgentStreamHandler) handleSessionTitle(ctx context.Context, evt event.Event) error {
	data, ok := evt.Data.(event.SessionTitleData)
	if !ok {
		return nil
	}

	// Use background context for title event since it may arrive after stream completion
	bgCtx := context.Background()

	// Append title event to stream
	if err := h.streamManager.AppendEvent(bgCtx, h.sessionID, h.assistantMessageID, interfaces.StreamEvent{
		ID:        evt.ID,
		Type:      types.ResponseTypeSessionTitle,
		Content:   data.Title,
		Done:      true,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"session_id": data.SessionID,
			"title":      data.Title,
		},
	}); err != nil {
		logger.GetLogger(h.ctx).Warn("Append session title event to stream failed (stream may have ended)", "error", err)
	}

	return nil
}
