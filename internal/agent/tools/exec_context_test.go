package tools

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestToolExecContextCarriesArtifactIdentity(t *testing.T) {
	ctx := WithToolExecContext(context.Background(), &ToolExecContext{
		AssistantMessageID: "message-1",
		ToolCallID:         "call-1",
	})

	identity, ok := types.ToolCallIdentityFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "message-1", identity.AssistantMessageID)
	require.Equal(t, "call-1", identity.ToolCallID)
}
