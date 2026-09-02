package skills

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

var testToolCallIdentity = types.ToolCallIdentity{
	AssistantMessageID: "message-1",
	ToolCallID:         "call-1",
}

func testToolCallContext(sessionID string) context.Context {
	ctx := types.WithSessionID(context.Background(), sessionID)
	return types.WithToolCallIdentity(ctx, testToolCallIdentity)
}

func testToolArtifactOutputDir(t *testing.T) string {
	t.Helper()
	outputDir, err := ToolArtifactOutputDir(testToolCallIdentity)
	require.NoError(t, err)
	return outputDir
}

func TestToolArtifactOutputDirUsesStableTypedIdentity(t *testing.T) {
	first, err := ToolArtifactOutputDir(testToolCallIdentity)
	require.NoError(t, err)
	second, err := ToolArtifactOutputDir(testToolCallIdentity)
	require.NoError(t, err)
	require.Equal(t, first, second)

	other, err := ToolArtifactOutputDir(types.ToolCallIdentity{
		AssistantMessageID: "message-1",
		ToolCallID:         "call-2",
	})
	require.NoError(t, err)
	require.NotEqual(t, first, other)
	require.Error(t, func() error {
		_, scopeErr := ToolArtifactOutputDir(types.ToolCallIdentity{})
		return scopeErr
	}())
}
