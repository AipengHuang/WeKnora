package repository

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestValidatePersistedModelProtocolRequiresEmbeddingProviderCapability(t *testing.T) {
	tests := []struct {
		name           string
		provider       provider.ProviderName
		baseURL        string
		supportsVision bool
		wantErr        bool
	}{
		{name: "missing provider", wantErr: true},
		{
			name:     "chat-only provider",
			provider: provider.ProviderDeepSeek,
			baseURL:  "https://api.deepseek.com/v1",
			wantErr:  true,
		},
		{name: "missing base URL", provider: provider.ProviderAliyun, wantErr: true},
		{
			name:     "Aliyun text protocol with native root",
			provider: provider.ProviderAliyun,
			baseURL:  "https://dashscope.aliyuncs.com",
			wantErr:  true,
		},
		{
			name:           "Aliyun multimodal protocol with compatible endpoint",
			provider:       provider.ProviderAliyun,
			baseURL:        "https://dashscope.aliyuncs.com/compatible-mode/v1",
			supportsVision: true,
			wantErr:        true,
		},
		{
			name:     "Aliyun text embedding provider",
			provider: provider.ProviderAliyun,
			baseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
			wantErr:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePersistedModelProtocol(&types.Model{
				Type:   types.ModelTypeEmbedding,
				Source: types.ModelSourceRemote,
				Parameters: types.ModelParameters{
					Provider:       string(test.provider),
					BaseURL:        test.baseURL,
					SupportsVision: test.supportsVision,
				},
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("unexpected validation result: %v", err)
			}
		})
	}
}
