package types

import "testing"

func TestParseExternalTenantCredentialRefRequiresCanonicalUUID(t *testing.T) {
	const canonical = "a8af976f-47bd-5a9f-a270-4e92361e9a9d"
	ref, err := ParseExternalTenantCredentialRef(canonical)
	if err != nil || ref.String() != canonical {
		t.Fatalf("ParseExternalTenantCredentialRef() = %q, %v", ref, err)
	}
	for _, value := range []string{
		"A8AF976F-47BD-5A9F-A270-4E92361E9A9D",
		" a8af976f-47bd-5a9f-a270-4e92361e9a9d",
		"credential-1",
	} {
		if _, err := ParseExternalTenantCredentialRef(value); err == nil {
			t.Fatalf("ParseExternalTenantCredentialRef(%q) accepted non-canonical input", value)
		}
	}
}
