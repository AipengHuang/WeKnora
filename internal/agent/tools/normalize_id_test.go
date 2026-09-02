package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanonicalToolCallID(t *testing.T) {
	t.Run("provider ID unchanged", func(t *testing.T) {
		result := CanonicalToolCallID(ToolCallCoordinates{ProviderID: "call_abc123"})
		assert.Equal(t, "call_abc123", result)
	})

	t.Run("missing ID uses stable call coordinates", func(t *testing.T) {
		coordinates := ToolCallCoordinates{
			AssistantMessageID: "message-1",
			Round:              1,
			Index:              0,
		}
		id1 := CanonicalToolCallID(coordinates)
		id2 := CanonicalToolCallID(coordinates)
		id3 := CanonicalToolCallID(ToolCallCoordinates{
			AssistantMessageID: "message-1",
			Round:              2,
			Index:              0,
		})

		assert.NotEmpty(t, id1)
		assert.Equal(t, id1, id2)
		assert.NotEqual(t, id1, id3)
	})

	t.Run("different assistant messages cannot collide", func(t *testing.T) {
		first := CanonicalToolCallID(ToolCallCoordinates{
			AssistantMessageID: "message-1",
			Round:              1,
			Index:              0,
		})
		second := CanonicalToolCallID(ToolCallCoordinates{
			AssistantMessageID: "message-2",
			Round:              1,
			Index:              0,
		})
		assert.NotEqual(t, first, second)
	})
}
