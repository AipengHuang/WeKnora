package types

import (
	"context"
	"testing"
)

func TestAuthorizeTenantAPIKeyKnowledgeTargetsRejectsKnowledgeIDs(t *testing.T) {
	ctx := WithTenantAPIKeyScope(context.Background(), TenantAPIKeyScope{
		KnowledgeBaseIDs: StringArray{"kb-1"},
	})
	err := AuthorizeTenantAPIKeyKnowledgeTargets(ctx, []string{"kb-1"}, []string{"doc-1"})
	if err == nil {
		t.Fatal("expected forbidden when knowledge_ids supplied under KB-restricted key")
	}
}

func TestAuthorizeTenantAPIKeyKnowledgeTargetsAllowsUnspecifiedTargets(t *testing.T) {
	ctx := WithTenantAPIKeyScope(context.Background(), TenantAPIKeyScope{
		KnowledgeBaseIDs: StringArray{"kb-1"},
	})
	err := AuthorizeTenantAPIKeyKnowledgeTargets(ctx, nil, nil)
	if err != nil {
		t.Fatalf("expected unspecified targets to be allowed, got %v", err)
	}
}

func TestAuthorizeTenantAPIKeyOptionalTagIDsRejectsTags(t *testing.T) {
	ctx := WithTenantAPIKeyScope(context.Background(), TenantAPIKeyScope{
		KnowledgeBaseIDs: StringArray{"kb-1"},
	})
	err := AuthorizeTenantAPIKeyOptionalTagIDs(ctx, []string{"tag-1"})
	if err == nil {
		t.Fatal("expected forbidden when tag_ids supplied under KB-restricted key")
	}
}

func TestFilterKnowledgeBasesForTenantAPIKeyScopeIntersectsAgentDefaults(t *testing.T) {
	ctx := WithTenantAPIKeyScope(context.Background(), TenantAPIKeyScope{
		KnowledgeBaseIDs: StringArray{"kb-1", "kb-2"},
	})
	got, err := FilterKnowledgeBasesForTenantAPIKeyScope(ctx, nil, []string{"kb-2", "kb-3"})
	if err != nil {
		t.Fatalf("FilterKnowledgeBasesForTenantAPIKeyScope returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "kb-2" {
		t.Fatalf("filtered = %#v, want only kb-2", got)
	}
}

func TestFilterKnowledgeBasesForTenantAPIKeyScopeRejectsExplicitOutOfScope(t *testing.T) {
	ctx := WithTenantAPIKeyScope(context.Background(), TenantAPIKeyScope{
		KnowledgeBaseIDs: StringArray{"kb-1"},
	})
	_, err := FilterKnowledgeBasesForTenantAPIKeyScope(ctx, []string{"kb-1", "kb-2"}, []string{"kb-1", "kb-2"})
	if err == nil {
		t.Fatal("expected forbidden for explicit out-of-scope kb_ids")
	}
}

func TestScopeHasCapability(t *testing.T) {
	s := TenantAPIKeyScope{Capabilities: StringArray{"chat"}}
	if !s.HasCapability(APIKeyCapabilityChat) {
		t.Fatal("expected chat capability to be present")
	}
	if (TenantAPIKeyScope{}).HasCapability(APIKeyCapabilityChat) {
		t.Fatal("empty scope must not report chat capability")
	}
	// 未声明的能力值不能获得授权。
	if s.HasCapability(APIKeyCapability("bogus")) {
		t.Fatal("unknown capability must not be satisfied")
	}
	if s.HasCapability(APIKeyCapability(" chat")) {
		t.Fatal("whitespace capability alias must not be satisfied")
	}
}

func TestParseAPIKeyCapabilitiesAcceptsDeclaredValues(t *testing.T) {
	input := StringArray{
		"retrieve",
		"chat",
		"read_agents",
		"manage_kbs",
		"message_history",
		"manage_mcp_services",
		"manage_sandboxes",
		"manage_members",
		"manage_spaces",
	}
	got, err := ParseAPIKeyCapabilities(input)
	if err != nil {
		t.Fatalf("ParseAPIKeyCapabilities returned error: %v", err)
	}
	want := []string{"retrieve", "chat", "read_agents", "manage_kbs", "message_history", "manage_mcp_services", "manage_sandboxes", "manage_members", "manage_spaces"}
	if len(got) != len(want) {
		t.Fatalf("parsed = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parsed = %#v, want %#v", got, want)
		}
	}
}

func TestParseAPIKeyCapabilitiesRejectsNonProtocolValues(t *testing.T) {
	tests := []struct {
		name   string
		values StringArray
	}{
		{name: "unknown", values: StringArray{"bogus"}},
		{name: "leading whitespace", values: StringArray{" chat"}},
		{name: "trailing whitespace", values: StringArray{"chat "}},
		{name: "wrong case", values: StringArray{"CHAT"}},
		{name: "empty", values: StringArray{""}},
		{name: "duplicate", values: StringArray{"chat", "chat"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseAPIKeyCapabilities(test.values); err == nil {
				t.Fatalf("ParseAPIKeyCapabilities(%#v) returned nil error", test.values)
			}
		})
	}
}
