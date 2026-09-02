package repository

import (
	"fmt"

	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/types"
)

// validatePersistedModelProtocol 阻止不完整的远程 Embedding 配置进入数据库。
func validatePersistedModelProtocol(model *types.Model) error {
	if model == nil {
		return fmt.Errorf("model is required")
	}
	if model.Source != types.ModelSourceRemote || model.Type != types.ModelTypeEmbedding {
		return nil
	}
	return embedding.ValidateRemoteEmbeddingConfig(embedding.ConfigFromModel(model, "", ""))
}
