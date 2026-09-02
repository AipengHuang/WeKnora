package types

import "testing"

func TestParseExternalTenantRefRequiresCanonicalUUID(t *testing.T) {
	canonical := "7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7"
	parsed, err := ParseExternalTenantRef(canonical)
	if err != nil || parsed.String() != canonical {
		t.Fatalf("parse canonical reference = %q, %v", parsed, err)
	}
	for _, value := range []string{
		"7B4A9B8E-0B65-5BB4-A07F-DC712921F3B7",
		" 7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7",
		"7b4a9b8e0b655bb4a07fdc712921f3b7",
		"account-1",
		"",
	} {
		if _, err := ParseExternalTenantRef(value); err == nil {
			t.Fatalf("ParseExternalTenantRef(%q) succeeded", value)
		}
	}
}
