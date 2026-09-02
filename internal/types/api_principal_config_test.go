package types

import (
	"encoding/json"
	stderrors "errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/utils"
)

const principalConfigTestAESKey = "0123456789abcdef0123456789abcdef"

// TestAPIPrincipalConfigSecretPersistence 验证签名密钥只以密文写入并能严格读取。
func TestAPIPrincipalConfigSecretPersistence(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", principalConfigTestAESKey)
	config := &APIPrincipalConfig{
		Mode:       APIPrincipalModeSignedToken,
		HMACSecret: "test-signing-secret",
	}

	value, err := config.Value()
	if err != nil {
		t.Fatalf("persist API principal config: %v", err)
	}
	encoded, ok := value.([]byte)
	if !ok {
		t.Fatalf("persisted value type = %T, want []byte", value)
	}
	var stored struct {
		HMACSecret string `json:"hmac_secret"`
	}
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatalf("decode persisted API principal config: %v", err)
	}
	if stored.HMACSecret == config.HMACSecret {
		t.Fatal("API principal secret was stored as plaintext")
	}

	var restored APIPrincipalConfig
	if err := restored.Scan(encoded); err != nil {
		t.Fatalf("restore API principal config: %v", err)
	}
	if restored.HMACSecret != config.HMACSecret {
		t.Fatal("restored API principal secret does not match")
	}
}

// TestAPIPrincipalConfigSecretRequiresEncryptionKey 验证缺少密钥时禁止写入明文。
func TestAPIPrincipalConfigSecretRequiresEncryptionKey(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "")
	value, err := (&APIPrincipalConfig{HMACSecret: "test-signing-secret"}).Value()
	if err == nil || value != nil {
		t.Fatalf("persist result = (%T, %v), want nil value and error", value, err)
	}
}

// TestAPIPrincipalConfigSecretReadFailsClosed 验证密文无法解密时明确失败。
func TestAPIPrincipalConfigSecretReadFailsClosed(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", principalConfigTestAESKey)
	value, err := (&APIPrincipalConfig{HMACSecret: "test-signing-secret"}).Value()
	if err != nil {
		t.Fatalf("persist API principal config: %v", err)
	}

	t.Setenv("SYSTEM_AES_KEY", "")
	var restored APIPrincipalConfig
	err = restored.Scan(value)
	if !stderrors.Is(err, utils.ErrEncryptedDataMissingKey) {
		t.Fatalf("restore error = %v, want missing encryption key", err)
	}

	t.Setenv("SYSTEM_AES_KEY", "abcdef0123456789abcdef0123456789")
	err = (&APIPrincipalConfig{}).Scan(value)
	if err == nil {
		t.Fatal("restore with a rotated encryption key must fail")
	}
}

// TestAPIPrincipalConfigPlaintextReadFailsClosed 验证明文记录不能进入运行时。
func TestAPIPrincipalConfigPlaintextReadFailsClosed(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", principalConfigTestAESKey)
	encoded := []byte(`{"mode":"signed_token","hmac_secret":"plaintext-signing-secret"}`)
	for _, value := range []any{encoded, string(encoded)} {
		var restored APIPrincipalConfig
		if err := restored.Scan(value); !stderrors.Is(err, utils.ErrStoredSecretEnvelopeRequired) {
			t.Fatalf("restore error = %v, want encrypted-envelope error", err)
		}
	}
}
