package utils

import (
	"errors"
	"testing"
)

func TestDecryptEncryptedStoredSecret(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		value, err := DecryptEncryptedStoredSecret("")
		if err != nil || value != "" {
			t.Fatalf("decrypt result = (%q, %v), want empty success", value, err)
		}
	})

	t.Run("encrypted", func(t *testing.T) {
		t.Setenv("SYSTEM_AES_KEY", testAESKey)
		stored, err := EncryptAESGCM("test-secret", []byte(testAESKey))
		if err != nil {
			t.Fatalf("encrypt secret: %v", err)
		}
		value, err := DecryptEncryptedStoredSecret(stored)
		if err != nil || value != "test-secret" {
			t.Fatalf("decrypt result = (%q, %v), want plaintext", value, err)
		}
	})

	t.Run("plaintext", func(t *testing.T) {
		t.Setenv("SYSTEM_AES_KEY", testAESKey)
		_, err := DecryptEncryptedStoredSecret("test-plaintext-secret")
		if !errors.Is(err, ErrStoredSecretEnvelopeRequired) {
			t.Fatalf("decrypt error = %v, want envelope error", err)
		}
	})

	t.Run("malformed envelope", func(t *testing.T) {
		t.Setenv("SYSTEM_AES_KEY", testAESKey)
		_, err := DecryptEncryptedStoredSecret("enc:v1:")
		if !errors.Is(err, ErrStoredSecretEnvelopeRequired) {
			t.Fatalf("decrypt error = %v, want envelope error", err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		stored, err := EncryptAESGCM("test-secret", []byte(testAESKey))
		if err != nil {
			t.Fatalf("encrypt secret: %v", err)
		}
		t.Setenv("SYSTEM_AES_KEY", "")
		_, err = DecryptEncryptedStoredSecret(stored)
		if !errors.Is(err, ErrEncryptedDataMissingKey) {
			t.Fatalf("decrypt error = %v, want missing key error", err)
		}
	})
}

func TestEncryptStoredSecret(t *testing.T) {
	t.Run("encrypted", func(t *testing.T) {
		t.Setenv("SYSTEM_AES_KEY", testAESKey)
		stored, err := EncryptStoredSecret("test-secret")
		if err != nil {
			t.Fatalf("EncryptStoredSecret returned error: %v", err)
		}
		plain, err := DecryptEncryptedStoredSecret(stored)
		if err != nil || plain != "test-secret" {
			t.Fatalf("decrypt result = (%q, %v), want plaintext", plain, err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		t.Setenv("SYSTEM_AES_KEY", "")
		if _, err := EncryptStoredSecret("test-secret"); !errors.Is(err, ErrSecretEncryptionKeyUnavailable) {
			t.Fatalf("EncryptStoredSecret error = %v, want missing key error", err)
		}
	})

	t.Run("malformed envelope", func(t *testing.T) {
		t.Setenv("SYSTEM_AES_KEY", testAESKey)
		if _, err := EncryptStoredSecret("enc:v1:not-valid-ciphertext"); err == nil {
			t.Fatal("EncryptStoredSecret accepted malformed ciphertext")
		}
	})
}
