package skills

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
)

// ExecuteScript 在当前工具调用的独立产物目录中执行 Skill 脚本。
func (m *Manager) ExecuteScript(
	ctx context.Context,
	skillName string,
	scriptPath string,
	args []string,
	stdin string,
) (*sandbox.ExecuteResult, error) {
	if !m.enabled {
		return nil, fmt.Errorf("skills are not enabled")
	}
	if !m.isSkillAllowed(skillName) {
		return nil, fmt.Errorf("skill not allowed: %s", skillName)
	}
	if m.sandboxMgr == nil {
		return nil, fmt.Errorf("sandbox is not configured")
	}

	source := m.resolveSource(skillName)
	basePath, err := source.GetSkillBasePath(skillName)
	if err != nil {
		return nil, err
	}
	logger.Info(ctx, "[Tool][ExecuteScript]:Prepare execution config")
	sessionID, _ := types.SessionIDFromContext(ctx)
	identity, ok := types.ToolCallIdentityFromContext(ctx)
	env, err := ArtifactEnvironmentFromContext(identity, ok)
	if err != nil {
		return nil, err
	}
	outputDir := env[artifactOutputEnvVar]
	applySessionPackagePath(env, skillName)
	fileStore := sessionFileStoreFromManager(m.sandboxMgr)
	if fileStore != nil {
		env[sessionInputEnvVar] = sandbox.SessionInputRoot
		if sessionID != "" {
			if err := fileStore.EnsureSessionDir(ctx, sessionID, outputDir); err != nil {
				logger.Warnf(ctx, "[Tool][ExecuteScript] pre-create output dir %s failed: %v", outputDir, err)
			}
		}
	}
	if m.envResolver != nil {
		resolved, missing, err := m.envResolver.ResolveEnv(ctx, skillName)
		if err != nil {
			return nil, err
		}
		if len(missing) > 0 {
			return nil, &MissingSkillEnvError{SkillName: skillName, Names: missing}
		}
		ApplyResolvedEnv(env, resolved)
	}

	config, err := buildSkillExecuteConfig(
		source, skillName, scriptPath, basePath, args, stdin, env, sessionID,
	)
	if err != nil {
		return nil, err
	}
	return m.sandboxMgr.Execute(ctx, config)
}

// buildSkillExecuteConfig 将 Skill 来源转换为沙箱执行配置。
func buildSkillExecuteConfig(
	source SkillSource,
	skillName, scriptPath, basePath string,
	args []string,
	stdin string,
	env map[string]string,
	sessionID string,
) (*sandbox.ExecuteConfig, error) {
	if workspace, ok := sandbox.RunnableWorkspaceScript(scriptPath); ok {
		return workspaceSkillExecuteConfig(
			source, skillName, workspace, basePath, args, stdin, env, sessionID,
		)
	}
	image, installed := source.(imageSkillSource)
	if !installed {
		file, err := source.LoadSkillFile(skillName, scriptPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load script: %w", err)
		}
		if !file.IsScript {
			return nil, fmt.Errorf("file is not an executable script: %s", scriptPath)
		}
		return &sandbox.ExecuteConfig{
			Script: file.Path, Args: args, WorkDir: basePath, Stdin: stdin,
			Env: env, SessionID: sessionID,
		}, nil
	}
	if !IsScript(scriptPath) {
		return nil, fmt.Errorf("file is not an executable script: %s", scriptPath)
	}
	remoteScript, err := image.RemoteScriptPath(skillName, scriptPath)
	if err != nil {
		return nil, err
	}
	env[skillDirEnvVar] = basePath
	return &sandbox.ExecuteConfig{
		RemoteScriptPath: remoteScript,
		Args:             args,
		Stdin:            stdin,
		Env:              env,
		SessionID:        sessionID,
	}, nil
}

// workspaceSkillExecuteConfig 使用已安装 Skill 的解释器执行工作区脚本。
func workspaceSkillExecuteConfig(
	source SkillSource,
	skillName, workspacePath, basePath string,
	args []string,
	stdin string,
	env map[string]string,
	sessionID string,
) (*sandbox.ExecuteConfig, error) {
	if !IsScript(workspacePath) {
		return nil, fmt.Errorf("file is not an executable script: %s", workspacePath)
	}
	if _, installed := source.(imageSkillSource); !installed {
		return nil, fmt.Errorf(
			"script_path %q is a session workspace file; this skill is not installed in the sandbox image, so execute_skill_script cannot attach its environment. Run it with shell_exec, or pass a skill-relative path such as scripts/foo.py",
			workspacePath,
		)
	}
	skillDir, ok := sandbox.ValidatedImageSkillDir(basePath)
	if !ok {
		return nil, fmt.Errorf("cannot run workspace script %q for skill %q: no installed skill directory", workspacePath, skillName)
	}
	env[skillDirEnvVar] = skillDir
	return &sandbox.ExecuteConfig{
		RemoteScriptPath: workspacePath,
		SkillDir:         skillDir,
		Args:             args,
		Stdin:            stdin,
		Env:              env,
		SessionID:        sessionID,
	}, nil
}

func sessionFileStoreFromManager(mgr sandbox.Manager) sandbox.SessionFileStore {
	provider, ok := mgr.(sandbox.SessionCapabilityProvider)
	if !ok || provider == nil {
		return nil
	}
	return provider.SessionFileStore()
}
