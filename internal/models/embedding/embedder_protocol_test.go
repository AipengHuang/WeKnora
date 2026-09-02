package embedding

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestNewEmbedderRequiresExplicitRemoteProvider(t *testing.T) {
	_, err := newEmbedder(Config{
		Source:    types.ModelSourceRemote,
		BaseURL:   "https://example.com/v1",
		ModelName: "text-embedding-v4",
	}, nil, nil)
	if err == nil {
		t.Fatal("expected missing provider to fail")
	}
}

func TestNewEmbedderRejectsProviderWithoutEmbeddingCapability(t *testing.T) {
	_, err := newEmbedder(Config{
		Source:    types.ModelSourceRemote,
		Provider:  string(provider.ProviderDeepSeek),
		BaseURL:   "https://api.deepseek.com/v1",
		ModelName: "deepseek-chat",
	}, nil, nil)
	if err == nil {
		t.Fatal("expected non-embedding provider to fail")
	}
}

func TestNewEmbedderRoutesAliyunByDeclaredVisionCapability(t *testing.T) {
	allowEmbeddingTestHosts(t, "example.com")

	textModel, err := newEmbedder(Config{
		Source:         types.ModelSourceRemote,
		Provider:       string(provider.ProviderAliyun),
		BaseURL:        "https://example.com/compatible-mode/v1",
		ModelName:      "model-name-containing-vision",
		APIKey:         "test-key",
		SupportsVision: false,
	}, nil, nil)
	if err != nil {
		t.Fatalf("create text embedder: %v", err)
	}
	if _, ok := textModel.(*OpenAIEmbedder); !ok {
		t.Fatalf("expected text protocol, got %T", textModel)
	}

	visionModel, err := newEmbedder(Config{
		Source:         types.ModelSourceRemote,
		Provider:       string(provider.ProviderAliyun),
		BaseURL:        "https://example.com",
		ModelName:      "opaque-model-id",
		APIKey:         "test-key",
		SupportsVision: true,
	}, nil, nil)
	if err != nil {
		t.Fatalf("create vision embedder: %v", err)
	}
	if _, ok := visionModel.(*AliyunEmbedder); !ok {
		t.Fatalf("expected multimodal protocol, got %T", visionModel)
	}
}

func TestNewEmbedderRejectsAliyunProtocolEndpointMismatch(t *testing.T) {
	allowEmbeddingTestHosts(t, "example.com")

	tests := []struct {
		name           string
		baseURL        string
		supportsVision bool
	}{
		{
			name:           "text protocol with native root",
			baseURL:        "https://example.com",
			supportsVision: false,
		},
		{
			name:           "multimodal protocol with compatible path",
			baseURL:        "https://example.com/compatible-mode/v1",
			supportsVision: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newEmbedder(Config{
				Source:         types.ModelSourceRemote,
				Provider:       string(provider.ProviderAliyun),
				BaseURL:        test.baseURL,
				ModelName:      "opaque-model-id",
				APIKey:         "test-key",
				SupportsVision: test.supportsVision,
			}, nil, nil)
			if err == nil {
				t.Fatal("expected protocol endpoint mismatch to fail")
			}
		})
	}
}
