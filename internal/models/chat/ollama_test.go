package chat

import (
	"net/http"
	"net/http/httptest"
	"testing"

	secutils "github.com/Tencent/WeKnora/internal/utils"
	ollamaapi "github.com/ollama/ollama/api"
)

func TestResolveImageForOllamaRejectsInternalURL(t *testing.T) {
	t.Setenv("SSRF_WHITELIST", "")
	secutils.ResetSSRFWhitelistForTest()
	t.Cleanup(secutils.ResetSSRFWhitelistForTest)

	if data := resolveImageForOllama("http://169.254.169.254/latest/meta-data/"); data != nil {
		t.Fatalf("resolveImageForOllama returned data for blocked internal URL")
	}
}

func TestResolveImageForOllamaBlocksRedirectToInternalURL(t *testing.T) {
	t.Setenv("SSRF_WHITELIST", "127.0.0.1")
	secutils.ResetSSRFWhitelistForTest()
	t.Cleanup(secutils.ResetSSRFWhitelistForTest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer server.Close()

	if data := resolveImageForOllama(server.URL); data != nil {
		t.Fatalf("resolveImageForOllama returned data after redirect to blocked internal URL")
	}
}

func TestOllamaToolCallConversionPreservesProviderIdentity(t *testing.T) {
	firstToolCall := ollamaapi.ToolCall{ID: "call_provider_first", Function: ollamaapi.ToolCallFunction{
		Index:     7,
		Name:      "proof",
		Arguments: ollamaapi.NewToolCallFunctionArguments(),
	}}
	secondToolCall := firstToolCall
	secondToolCall.ID = "call_provider_second"
	chat := &OllamaChat{}

	first, err := chat.toolCallTo([]ollamaapi.ToolCall{firstToolCall})
	if err != nil {
		t.Fatal(err)
	}
	second, err := chat.toolCallTo([]ollamaapi.ToolCall{secondToolCall})
	if err != nil {
		t.Fatal(err)
	}
	if first[0].ID != firstToolCall.ID {
		t.Fatalf("first ID = %q, want %q", first[0].ID, firstToolCall.ID)
	}
	if second[0].ID != secondToolCall.ID {
		t.Fatalf("second ID = %q, want %q", second[0].ID, secondToolCall.ID)
	}

	replayed, err := chat.toolCallFrom([]ToolCall{{
		ID:               first[0].ID,
		Function:         FunctionCall{Name: "proof", Arguments: "{}"},
		ProviderMetadata: first[0].ProviderMetadata,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if replayed[0].ID != first[0].ID || replayed[0].Function.Index != 7 {
		t.Fatalf("replayed tool call = %#v", replayed[0])
	}
}

func TestOllamaToolCallConversionRejectsMissingProviderIdentity(t *testing.T) {
	chat := &OllamaChat{}
	_, err := chat.toolCallTo([]ollamaapi.ToolCall{{Function: ollamaapi.ToolCallFunction{
		Index:     0,
		Name:      "proof",
		Arguments: ollamaapi.NewToolCallFunctionArguments(),
	}}})
	if err == nil {
		t.Fatal("tool call without provider ID was accepted")
	}
}

func TestOllamaBuildChatRequestRejectsInvalidToolSchema(t *testing.T) {
	chat := &OllamaChat{}
	_, err := chat.buildChatRequest(nil, &ChatOptions{Tools: []Tool{{
		Type: "function",
		Function: FunctionDef{
			Name:       "proof",
			Parameters: []byte(`{"type":`),
		},
	}}}, false)
	if err == nil {
		t.Fatal("invalid tool schema was accepted")
	}
}

func TestOllamaMessageTextKeepsThinkingSeparate(t *testing.T) {
	content, reasoning := ollamaMessageText(ollamaapi.Message{Thinking: "private reasoning"})
	if content != "" {
		t.Fatalf("thinking was exposed as answer content: %q", content)
	}
	if reasoning != "private reasoning" {
		t.Fatalf("reasoning = %q, want private reasoning", reasoning)
	}
}
