package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"

	"github.com/Tencent/WeKnora/internal/types"
)

const toolArtifactDirectory = "tool-calls"

// ToolArtifactOutputDir 为一个明确的工具调用生成不可碰撞的产物目录。
func ToolArtifactOutputDir(identity types.ToolCallIdentity) (string, error) {
	return ToolArtifactOutputDirAt(ArtifactOutputDir(), identity)
}

// ToolArtifactOutputDirAt 在显式根目录下生成工具产物目录。
func ToolArtifactOutputDirAt(root string, identity types.ToolCallIdentity) (string, error) {
	if !identity.IsValid() {
		return "", fmt.Errorf("tool call identity is required for artifact output")
	}
	if !path.IsAbs(root) {
		return "", fmt.Errorf("artifact output root must be absolute")
	}
	hash := sha256.Sum256([]byte(identity.AssistantMessageID + "\x00" + identity.ToolCallID))
	return path.Join(path.Clean(root), toolArtifactDirectory, hex.EncodeToString(hash[:])), nil
}

// ArtifactEnvironment 返回工具执行必须使用的标准产物环境。
func ArtifactEnvironment(identity types.ToolCallIdentity) (map[string]string, error) {
	outputDir, err := ToolArtifactOutputDir(identity)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		artifactOutputEnvVar:  outputDir,
		artifactHistoryEnvVar: ArtifactOutputDir(),
	}, nil
}

// ArtifactEnvironmentFromContext 从工具上下文生成标准产物环境。
func ArtifactEnvironmentFromContext(identity types.ToolCallIdentity, ok bool) (map[string]string, error) {
	if !ok {
		return nil, fmt.Errorf("tool call identity is required for artifact output")
	}
	return ArtifactEnvironment(identity)
}
