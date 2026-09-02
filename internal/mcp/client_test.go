package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/mark3labs/mcp-go/client/transport"
	protocol "github.com/mark3labs/mcp-go/mcp"
)

func decodeJSONObject(t *testing.T, value json.RawMessage) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(value, &decoded); err != nil {
		t.Fatalf("failed to decode JSON object: %v", err)
	}
	return decoded
}

func TestConvertProtocolToolPreservesStandardDefinition(t *testing.T) {
	readOnly := true
	tool := protocol.Tool{
		Name: "render_card", Description: "Render a card",
		RawInputSchema:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
		RawOutputSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}}}`),
		Annotations:     protocol.ToolAnnotation{Title: "Card", ReadOnlyHint: &readOnly},
		Meta: protocol.NewMetaFromMap(map[string]any{
			"ui": map[string]any{"resourceUri": "ui://cards/card/v1/index.html"},
		}),
	}
	converted, err := convertProtocolTool(tool)
	if err != nil {
		t.Fatalf("convertProtocolTool returned error: %v", err)
	}
	if decodeJSONObject(t, converted.Meta)["ui"] == nil {
		t.Fatal("tool _meta was not preserved")
	}
	if decodeJSONObject(t, converted.OutputSchema)["type"] != "object" {
		t.Fatal("outputSchema was not preserved")
	}
	definition := decodeJSONObject(t, converted.Definition)
	if definition["_meta"] == nil || definition["annotations"] == nil {
		t.Fatal("complete tool definition was not preserved")
	}
}

func TestConvertProtocolCallResultPreservesStructuredFields(t *testing.T) {
	result := protocol.NewToolResultStructured(
		map[string]any{"artifact": map[string]any{"referenceId": "ref-1"}},
		"Created artifact",
	)
	result.Meta = protocol.NewMetaFromMap(map[string]any{"traceId": "trace-1"})
	converted, err := convertProtocolCallResult(result)
	if err != nil {
		t.Fatalf("convertProtocolCallResult returned error: %v", err)
	}
	if decodeJSONObject(t, converted.StructuredContent)["artifact"] == nil {
		t.Fatal("structuredContent was not preserved")
	}
	if decodeJSONObject(t, converted.Meta)["traceId"] != "trace-1" {
		t.Fatal("result _meta was not preserved")
	}
	var content []map[string]any
	if err := json.Unmarshal(converted.RawContent, &content); err != nil || len(content) != 1 {
		t.Fatalf("content was not preserved: %v", err)
	}
}

func TestConvertProtocolCallResultRejectsNonObjectStructuredContent(t *testing.T) {
	_, err := convertProtocolCallResult(&protocol.CallToolResult{
		Content:           []protocol.Content{protocol.NewTextContent("invalid")},
		StructuredContent: []string{"invalid"},
	})
	if err == nil {
		t.Fatal("expected non-object structuredContent to be rejected")
	}
}

func TestAsOAuthRequired(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if got := asOAuthRequired(nil); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("401 with RFC 9728 metadata is treated as OAuth required", func(t *testing.T) {
		meta := "https://example.com/.well-known/oauth-protected-resource"
		err := fmt.Errorf("wrap: %w", &transport.AuthorizationRequiredError{ResourceMetadataURL: meta})
		got := asOAuthRequired(err)
		if got == nil {
			t.Fatal("expected non-nil OAuthRequiredError")
		}
		if got.MetadataURL != meta {
			t.Errorf("MetadataURL = %q, want %q", got.MetadataURL, meta)
		}
	})

	t.Run("bare 401 without metadata is NOT OAuth required", func(t *testing.T) {
		err := &transport.AuthorizationRequiredError{ResourceMetadataURL: ""}
		if got := asOAuthRequired(err); got != nil {
			t.Fatalf("got %v, want nil (bare 401 should not suggest OAuth)", got)
		}
	})

	t.Run("unrelated error is ignored", func(t *testing.T) {
		if got := asOAuthRequired(errors.New("connection refused")); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})
}

func TestApplyAuthHeaders(t *testing.T) {
	tests := []struct {
		name string
		ac   *types.MCPAuthConfig
		want map[string]string
	}{
		{
			name: "nil config injects nothing",
			ac:   nil,
			want: map[string]string{},
		},
		{
			name: "api_key uses default X-API-Key header",
			ac:   &types.MCPAuthConfig{AuthType: types.MCPAuthAPIKey, APIKey: "k1"},
			want: map[string]string{"X-API-Key": "k1"},
		},
		{
			name: "api_key honors custom header name (e.g. raw token in Authorization)",
			ac: &types.MCPAuthConfig{
				AuthType:     types.MCPAuthAPIKey,
				APIKey:       "f7bfde",
				APIKeyHeader: "Authorization",
			},
			want: map[string]string{"Authorization": "f7bfde"},
		},
		{
			name: "bearer adds Bearer prefix",
			ac:   &types.MCPAuthConfig{AuthType: types.MCPAuthBearer, Token: "t1"},
			want: map[string]string{"Authorization": "Bearer t1"},
		},
		{
			name: "selected strategy is exclusive — stale token is not emitted",
			ac: &types.MCPAuthConfig{
				AuthType: types.MCPAuthAPIKey,
				APIKey:   "k1",
				Token:    "stale",
			},
			want: map[string]string{"X-API-Key": "k1"},
		},
		{
			name: "empty AuthType keeps legacy behavior (infer from fields)",
			ac: &types.MCPAuthConfig{
				AuthType: types.MCPAuthNone,
				APIKey:   "k1",
				Token:    "t1",
			},
			want: map[string]string{"X-API-Key": "k1", "Authorization": "Bearer t1"},
		},
		{
			name: "custom headers are always layered on top",
			ac: &types.MCPAuthConfig{
				AuthType:      types.MCPAuthBearer,
				Token:         "t1",
				CustomHeaders: map[string]string{"X-Trace": "abc"},
			},
			want: map[string]string{"Authorization": "Bearer t1", "X-Trace": "abc"},
		},
		{
			name: "oauth strategy emits no static header (handled elsewhere)",
			ac:   &types.MCPAuthConfig{AuthType: types.MCPAuthOAuth, Token: "ignored"},
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{}
			applyAuthHeaders(headers, tt.ac)
			if len(headers) != len(tt.want) {
				t.Fatalf("header count = %d, want %d (%v)", len(headers), len(tt.want), headers)
			}
			for k, v := range tt.want {
				if headers[k] != v {
					t.Errorf("header[%q] = %q, want %q", k, headers[k], v)
				}
			}
		})
	}
}
