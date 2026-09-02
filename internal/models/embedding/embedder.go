package embedding

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/Tencent/WeKnora/internal/models/utils/ollama"
	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
	"github.com/Tencent/WeKnora/internal/types"
)

// Embedder defines the interface for text vectorization
type Embedder interface {
	// Embed converts text to vector
	Embed(ctx context.Context, text string) ([]float32, error)

	// BatchEmbed converts multiple texts to vectors in batch
	BatchEmbed(ctx context.Context, texts []string) ([][]float32, error)

	// GetModelName returns the model name
	GetModelName() string

	// GetDimensions returns the vector dimensions
	GetDimensions() int

	// GetModelID returns the model ID
	GetModelID() string

	EmbedderPooler
}

type EmbedderPooler interface {
	BatchEmbedWithPool(ctx context.Context, model Embedder, texts []string) ([][]float32, error)
}

// EmbedderType represents the embedder type
type EmbedderType string

// Config represents the embedder configuration
type Config struct {
	Source                    types.ModelSource `json:"source"`
	BaseURL                   string            `json:"base_url"`
	ModelName                 string            `json:"model_name"`
	APIKey                    string            `json:"api_key"`
	TruncatePromptTokens      int               `json:"truncate_prompt_tokens"`
	Dimensions                int               `json:"dimensions"`
	SupportsDimensionOverride bool              `json:"supports_dimension_override"`
	ModelID                   string            `json:"model_id"`
	Provider                  string            `json:"provider"`
	// MaxConcurrency caps concurrent background calls to this model; 0 falls
	// back to the process-wide default (see limiter.GateN).
	MaxConcurrency int               `json:"max_concurrency"`
	ExtraConfig    map[string]string `json:"extra_config"`
	// CustomHeaders 允许在调用远程 API 时附加自定义 HTTP 请求头（类似 OpenAI Python SDK 的 extra_headers）。
	CustomHeaders  map[string]string `json:"custom_headers"`
	SupportsVision bool
	AppID          string
	AppSecret      string // 加密值，工厂函数调用方传入，使用前已解密
}

// ConfigFromModel 根据 types.Model 构造 embedding.Config。
// 生产路径（从 DB 拉起）和测试连接路径（临时表单）共享这份映射。
// appID / appSecret 是已解密的 WeKnoraCloud 凭证，调用方负责传入。
func ConfigFromModel(m *types.Model, appID, appSecret string) Config {
	if m == nil {
		return Config{}
	}
	return Config{
		Source:                    m.Source,
		BaseURL:                   m.Parameters.BaseURL,
		APIKey:                    m.Parameters.APIKey,
		ModelID:                   m.ID,
		ModelName:                 m.Name,
		Dimensions:                m.Parameters.EmbeddingParameters.Dimension,
		SupportsDimensionOverride: m.Parameters.EmbeddingParameters.SupportsDimensionOverride,
		TruncatePromptTokens:      m.Parameters.EmbeddingParameters.TruncatePromptTokens,
		Provider:                  m.Parameters.Provider,
		MaxConcurrency:            m.Parameters.MaxConcurrency,
		ExtraConfig:               m.Parameters.ExtraConfig,
		CustomHeaders:             m.Parameters.CustomHeaders,
		SupportsVision:            m.Parameters.SupportsVision,
		AppID:                     appID,
		AppSecret:                 appSecret,
	}
}

// NewEmbedder creates an embedder based on the configuration
func NewEmbedder(config Config, pooler EmbedderPooler, ollamaService *ollama.OllamaService) (Embedder, error) {
	e, err := newEmbedder(config, pooler, ollamaService)
	if err != nil {
		return e, err
	}
	if setter, ok := e.(interface{ SetSupportsDimensionOverride(bool) }); ok {
		setter.SetSupportsDimensionOverride(config.SupportsDimensionOverride)
	}
	// Innermost: gate the real provider round-trips (including the per-sub-batch
	// pool callbacks) before debug/langfuse wrap for logging/tracing. See
	// concurrencyEmbedder for why this sits below the observability decorators.
	e = wrapEmbeddingConcurrency(e, config.MaxConcurrency)
	if logger.LLMDebugEnabled() {
		e = &debugEmbedder{inner: e}
	}
	if langfuse.GetManager().Enabled() {
		e = &langfuseEmbedder{inner: e}
	}
	return e, nil
}

func newEmbedder(config Config, pooler EmbedderPooler, ollamaService *ollama.OllamaService) (Embedder, error) {
	var embedder Embedder
	var err error
	switch config.Source {
	case types.ModelSourceLocal:
		embedder, err = NewOllamaEmbedder(config.BaseURL,
			config.ModelName, config.TruncatePromptTokens, config.Dimensions, config.ModelID, pooler, ollamaService)
		return embedder, err
	case types.ModelSourceRemote:
		if err := ValidateRemoteEmbeddingConfig(config); err != nil {
			return nil, err
		}
		providerName := provider.ProviderName(config.Provider)

		switch providerName {
		case provider.ProviderAliyun:
			if config.SupportsVision {
				aliyunEmb, aErr := NewAliyunEmbedder(config.APIKey,
					config.BaseURL,
					config.ModelName,
					config.TruncatePromptTokens,
					config.Dimensions,
					config.ModelID,
					pooler)
				if aliyunEmb != nil {
					aliyunEmb.SetCustomHeaders(config.CustomHeaders)
				}
				embedder, err = aliyunEmb, aErr
			} else {
				openaiEmb, oErr := NewOpenAIEmbedder(config.APIKey,
					config.BaseURL,
					config.ModelName,
					config.Dimensions,
					config.ModelID,
					pooler)
				if openaiEmb != nil {
					openaiEmb.SetCustomHeaders(config.CustomHeaders)
				}
				embedder, err = openaiEmb, oErr
			}
			return embedder, err
		case provider.ProviderVolcengine:
			// Volcengine Ark uses multimodal embedding API
			volcEmb, vErr := NewVolcengineEmbedder(config.APIKey,
				config.BaseURL,
				config.ModelName,
				config.TruncatePromptTokens,
				config.Dimensions,
				config.ModelID,
				pooler)
			if volcEmb != nil {
				volcEmb.SetCustomHeaders(config.CustomHeaders)
			}
			embedder, err = volcEmb, vErr
			return embedder, err
		case provider.ProviderJina:
			// Jina AI uses different API format (truncate instead of truncate_prompt_tokens)
			jinaEmb, jErr := NewJinaEmbedder(config.APIKey,
				config.BaseURL,
				config.ModelName,
				config.TruncatePromptTokens,
				config.Dimensions,
				config.ModelID,
				pooler)
			if jinaEmb != nil {
				jinaEmb.SetCustomHeaders(config.CustomHeaders)
			}
			embedder, err = jinaEmb, jErr
			return embedder, err
		case provider.ProviderAzureOpenAI:
			apiVersion := "2024-10-21"
			if config.ExtraConfig != nil {
				if v, ok := config.ExtraConfig["api_version"]; ok {
					apiVersion = v
				}
			}
			azureEmb, azErr := NewAzureOpenAIEmbedder(config.APIKey,
				config.BaseURL,
				config.ModelName,
				config.TruncatePromptTokens,
				config.Dimensions,
				config.ModelID,
				apiVersion,
				pooler)
			if azureEmb != nil {
				azureEmb.SetCustomHeaders(config.CustomHeaders)
			}
			embedder, err = azureEmb, azErr
			return embedder, err
		case provider.ProviderNvidia:
			nvEmb, nErr := NewNvidiaEmbedder(config.APIKey,
				config.BaseURL,
				config.ModelName,
				config.Dimensions,
				config.ModelID,
				pooler)
			if nvEmb != nil {
				nvEmb.SetCustomHeaders(config.CustomHeaders)
			}
			embedder, err = nvEmb, nErr
			return embedder, err
		case provider.ProviderGemini:
			geminiEmb, gErr := NewGeminiEmbedder(config.APIKey,
				config.BaseURL,
				config.ModelName,
				config.TruncatePromptTokens,
				config.Dimensions,
				config.ModelID,
				pooler)
			if geminiEmb != nil {
				geminiEmb.SetCustomHeaders(config.CustomHeaders)
			}
			embedder, err = geminiEmb, gErr
			return embedder, err
		case provider.ProviderZhipu:
			zhipuEmb, zErr := NewZhipuEmbedder(config.APIKey,
				config.BaseURL,
				config.ModelName,
				config.TruncatePromptTokens,
				config.Dimensions,
				config.ModelID,
				pooler)
			if zhipuEmb != nil {
				zhipuEmb.SetCustomHeaders(config.CustomHeaders)
			}
			embedder, err = zhipuEmb, zErr
			return embedder, err
		case provider.ProviderWeKnoraCloud:
			embedder, err = NewWeKnoraCloudEmbedder(config)
			return embedder, err
		default:
			// 其余已声明支持 Embedding 的 Provider 使用 OpenAI 标准接口。
			openaiEmb, oErr := NewOpenAIEmbedder(config.APIKey,
				config.BaseURL,
				config.ModelName,
				config.Dimensions,
				config.ModelID,
				pooler)
			if openaiEmb != nil {
				openaiEmb.SetCustomHeaders(config.CustomHeaders)
			}
			embedder, err = openaiEmb, oErr
			return embedder, err
		}
	default:
		return nil, fmt.Errorf("unsupported embedder source: %s", config.Source)
	}
}

// ValidateRemoteEmbeddingConfig 在保存和运行前执行同一套显式协议校验。
func ValidateRemoteEmbeddingConfig(config Config) error {
	providerName := provider.ProviderName(config.Provider)
	if providerName == "" {
		return fmt.Errorf("provider is required for remote embedding models")
	}
	registeredProvider, ok := provider.Get(providerName)
	if !ok || !slices.Contains(registeredProvider.Info().ModelTypes, types.ModelTypeEmbedding) {
		return fmt.Errorf("provider %q does not support embedding models", providerName)
	}
	if config.BaseURL == "" {
		return fmt.Errorf("base URL is required for remote embedding provider %q", providerName)
	}
	if providerName == provider.ProviderAliyun {
		return validateAliyunEmbeddingBaseURL(config.BaseURL, config.SupportsVision)
	}
	return nil
}
