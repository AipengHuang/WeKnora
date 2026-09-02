package embedding

import (
	"testing"

	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// allowEmbeddingTestHosts 只在当前非并行测试期间安装精确白名单。
func allowEmbeddingTestHosts(t *testing.T, hosts string) {
	t.Helper()
	secutils.ResetSSRFWhitelistForTest()
	secutils.SetSSRFWhitelistFromRaw(hosts)
	t.Cleanup(secutils.ResetSSRFWhitelistForTest)
}
