package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIEmbedderBatchEmbedOmitsDimensionsByDefault(t *testing.T) {
	requestBody := captureOpenAIEmbeddingRequest(t, "text-embedding-3-small", 256, false)

	if _, ok := requestBody["dimensions"]; ok {
		t.Fatalf("expected request body to omit dimensions by default, got %v", requestBody)
	}
}

func TestOpenAIEmbedderBatchEmbedSendsDimensionsWhenOverrideEnabled(t *testing.T) {
	requestBody := captureOpenAIEmbeddingRequest(t, "text-embedding-3-small", 256, true)

	got, ok := requestBody["dimensions"]
	if !ok {
		t.Fatalf("expected request body to include dimensions, got %v", requestBody)
	}
	if got != float64(256) {
		t.Fatalf("unexpected dimensions value: got %v want 256", got)
	}
}

func TestOpenAIEmbedderBatchEmbedOmitsDimensionsForOpenAICompatibleModels(t *testing.T) {
	requestBody := captureOpenAIEmbeddingRequest(t, "text-embedding-v3", 1024, false)

	if _, ok := requestBody["dimensions"]; ok {
		t.Fatalf("expected request body to omit dimensions for OpenAI-compatible model, got %v", requestBody)
	}
}

func TestOpenAIEmbedderBatchEmbedOmitsDimensionsForFixedSizeModels(t *testing.T) {
	requestBody := captureOpenAIEmbeddingRequest(t, "text-embedding-ada-002", 1536, false)

	if _, ok := requestBody["dimensions"]; ok {
		t.Fatalf("expected request body to omit dimensions for fixed-size model, got %v", requestBody)
	}
}

func TestOpenAIEmbedderBatchEmbedUsesOnlyStandardFields(t *testing.T) {
	requestBody := captureOpenAIEmbeddingRequest(t, "text-embedding-v4", 1024, true)

	if _, ok := requestBody["truncate_prompt_tokens"]; ok {
		t.Fatalf("expected standard request body to omit non-standard truncation, got %v", requestBody)
	}
}

func TestOrderOpenAIEmbeddingsUsesResponseIndexes(t *testing.T) {
	got, err := orderOpenAIEmbeddings([]OpenAIEmbeddingData{
		{Index: 1, Embedding: []float32{0.3, 0.4}},
		{Index: 0, Embedding: []float32{0.1, 0.2}},
	}, 2, 2)
	if err != nil {
		t.Fatalf("order embeddings: %v", err)
	}
	if got[0][0] != 0.1 || got[1][0] != 0.3 {
		t.Fatalf("unexpected embedding order: %v", got)
	}
}

func TestOrderOpenAIEmbeddingsRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name       string
		data       []OpenAIEmbeddingData
		inputCount int
		dimensions int
		wantErr    string
	}{
		{
			name:       "out of range index",
			data:       []OpenAIEmbeddingData{{Index: 1, Embedding: []float32{0.1}}},
			inputCount: 1,
			dimensions: 1,
			wantErr:    "embedding response index 1 is outside input range 0..0",
		},
		{
			name: "duplicate index",
			data: []OpenAIEmbeddingData{
				{Index: 0, Embedding: []float32{0.1}},
				{Index: 0, Embedding: []float32{0.2}},
			},
			inputCount: 2,
			dimensions: 1,
			wantErr:    "embedding response contains duplicate index 0",
		},
		{
			name:       "missing index",
			data:       []OpenAIEmbeddingData{{Index: 0, Embedding: []float32{0.1}}},
			inputCount: 2,
			dimensions: 1,
			wantErr:    "embedding response is missing index 1",
		},
		{
			name:       "wrong dimension",
			data:       []OpenAIEmbeddingData{{Index: 0, Embedding: []float32{0.1}}},
			inputCount: 1,
			dimensions: 2,
			wantErr:    "embedding response index 0 has dimension 1, expected 2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := orderOpenAIEmbeddings(test.data, test.inputCount, test.dimensions)
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("unexpected error: got %v want %q", err, test.wantErr)
			}
		})
	}
}

func captureOpenAIEmbeddingRequest(
	t *testing.T,
	modelName string,
	dimensions int,
	supportsDimensionOverride bool,
) map[string]any {
	t.Helper()
	allowEmbeddingTestHosts(t, "127.0.0.1")

	requestBody := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		vector := make([]float32, dimensions)
		if len(vector) > 0 {
			vector[0] = 0.1
		}
		if err := json.NewEncoder(w).Encode(OpenAIEmbedResponse{Data: []OpenAIEmbeddingData{{
			Embedding: vector,
			Index:     0,
		}}}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	embedder, err := NewOpenAIEmbedder(
		"test-key",
		server.URL,
		modelName,
		dimensions,
		"8f7d6082-5a15-4f84-ae55-88b2bdac4ba0",
		nil,
	)
	if err != nil {
		t.Fatalf("NewOpenAIEmbedder: %v", err)
	}
	embedder.SetSupportsDimensionOverride(supportsDimensionOverride)

	if _, err := embedder.BatchEmbed(context.Background(), []string{"hello"}); err != nil {
		t.Fatalf("BatchEmbed: %v", err)
	}

	return requestBody
}
