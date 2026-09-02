package tools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillScriptCommandDoesNotJoinWorkspacePaths(t *testing.T) {
	got := skillScriptCommand(ExecuteSkillScriptInput{
		SkillName:  "ppt-generator",
		ScriptPath: "/workspace/output/generate_rencui_ppt.py",
		Args:       []string{"--flag"},
	})
	require.Equal(t, "/workspace/output/generate_rencui_ppt.py --flag", got)
	assert.NotContains(t, got, "ppt-generator/")
}

func TestSkillScriptCommandKeepsRelativeSkillPaths(t *testing.T) {
	got := skillScriptCommand(ExecuteSkillScriptInput{
		SkillName:  "pdf",
		ScriptPath: "scripts/extract.py",
	})
	require.Equal(t, "pdf/scripts/extract.py", got)
}

func TestExecuteSkillScriptDescriptionAcceptsWorkspacePaths(t *testing.T) {
	assert.Contains(t, executeSkillScriptTool.Description(), "write_sandbox_file")
	assert.Contains(t, executeSkillScriptTool.Description(), "edit_sandbox_file")
	assert.Contains(t, executeSkillScriptTool.Description(), "/workspace/output")
	assert.Contains(t, executeSkillScriptTool.Description(), "/workspace/input")
	assert.Contains(t, executeSkillScriptTool.Description(), "install_deps.py")
	assert.Contains(t, executeSkillScriptTool.Description(), ".skill-packages")
}

func TestExecuteSkillScriptInputRequiresArgumentArray(t *testing.T) {
	var input ExecuteSkillScriptInput
	err := json.Unmarshal([]byte(`{"skill_name":"proof","script_path":"scripts/run.py","args":"--flag"}`), &input)
	require.Error(t, err)

	err = json.Unmarshal([]byte(`{"skill_name":"proof","script_path":"scripts/run.py","args":["--flag"]}`), &input)
	require.NoError(t, err)
	require.Equal(t, []string{"--flag"}, input.Args)
}
