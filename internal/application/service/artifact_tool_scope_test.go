package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

type scopedArtifactSource struct {
	requestedDir string
	entry        sandbox.RemoteDirEntry
}

func (source *scopedArtifactSource) ListSessionFiles(
	_ context.Context,
	_ string,
	dir string,
) ([]sandbox.RemoteDirEntry, error) {
	source.requestedDir = dir
	return []sandbox.RemoteDirEntry{source.entry}, nil
}

func (source *scopedArtifactSource) ReadSessionFile(
	_ context.Context,
	_ string,
	_ string,
) ([]byte, error) {
	return []byte("report"), nil
}

func TestArtifactCollectorBindsExactToolCallScope(t *testing.T) {
	identity := types.ToolCallIdentity{
		AssistantMessageID: "message-1",
		ToolCallID:         "call-1",
	}
	expectedDir, err := skills.ToolArtifactOutputDir(identity)
	require.NoError(t, err)
	source := &scopedArtifactSource{entry: sandbox.RemoteDirEntry{
		Name:    "report.txt",
		Path:    expectedDir + "/report.txt",
		Type:    sandbox.RemoteEntryFile,
		Size:    6,
		ModTime: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
	}}
	collector := NewArtifactCollector(
		source,
		&fakeFileService{},
		&fakeStore{},
		nil,
		ArtifactCollectorConfig{},
	)

	artifacts, err := collector.Collect(
		context.Background(),
		"session-1",
		identity.AssistantMessageID,
		1,
		identity.ToolCallID,
	)

	require.NoError(t, err)
	require.Equal(t, expectedDir, source.requestedDir)
	require.Len(t, artifacts, 1)
	require.Equal(t, identity.ToolCallID, artifacts[0].ToolCallID)
}
