package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type recordingCompletionStreamManager struct {
	events []interfaces.StreamEvent
}

func (m *recordingCompletionStreamManager) AppendEvent(
	_ context.Context,
	_, _ string,
	evt interfaces.StreamEvent,
) error {
	m.events = append(m.events, evt)
	return nil
}

func (*recordingCompletionStreamManager) GetEvents(
	context.Context,
	string,
	string,
	int,
) ([]interfaces.StreamEvent, int, error) {
	return nil, 0, nil
}

func TestHandleCompleteDoesNotSynthesizeAnswerEvents(t *testing.T) {
	streamManager := &recordingCompletionStreamManager{}
	message := &types.Message{ID: "message-1"}
	handler := &AgentStreamHandler{
		ctx:                context.Background(),
		sessionID:          "session-1",
		assistantMessageID: message.ID,
		assistantMessage:   message,
		streamManager:      streamManager,
	}

	err := handler.handleComplete(context.Background(), event.Event{
		ID: "complete-1",
		Data: event.AgentCompleteData{
			MessageID:   message.ID,
			FinalAnswer: "provider completion text",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "provider completion text", message.Content)
	require.Len(t, streamManager.events, 1)
	require.Equal(t, types.ResponseTypeComplete, streamManager.events[0].Type)
}
