package types

import "testing"

func TestTenantAPIKeyScopeNormalizePreservesFullAccess(t *testing.T) {
	scope := TenantAPIKeyScope{FullAccess: true}.Normalize()
	if !scope.FullAccess {
		t.Fatal("normalized scope lost full access")
	}
}

func TestTenantAPIKeyScopeNormalizeDefaultsToScopedAccess(t *testing.T) {
	scope := TenantAPIKeyScope{}.Normalize()
	if scope.FullAccess {
		t.Fatal("empty scope must not become full access")
	}
}

func TestTenantAPIKeyScopeNormalizeDoesNotRewriteCapabilities(t *testing.T) {
	scope := TenantAPIKeyScope{
		Capabilities: StringArray{"chat", "retrieve"},
	}.Normalize()
	want := StringArray{"chat", "retrieve"}
	if len(scope.Capabilities) != len(want) {
		t.Fatalf("capabilities = %#v, want %#v", scope.Capabilities, want)
	}
	for i := range want {
		if scope.Capabilities[i] != want[i] {
			t.Fatalf("capabilities = %#v, want %#v", scope.Capabilities, want)
		}
	}
}

func TestParseAPIKeyScopeTypeRejectsAliases(t *testing.T) {
	for _, value := range []APIKeyScopeType{"", "tenant ", "TENANT", "unknown"} {
		if _, err := ParseAPIKeyScopeType(value); err == nil {
			t.Fatalf("ParseAPIKeyScopeType(%q) returned nil error", value)
		}
	}
	for _, value := range []APIKeyScopeType{APIKeyScopeTenant, APIKeyScopePlatform} {
		parsed, err := ParseAPIKeyScopeType(value)
		if err != nil || parsed != value {
			t.Fatalf("ParseAPIKeyScopeType(%q) = (%q, %v)", value, parsed, err)
		}
	}
}
