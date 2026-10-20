package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSignAndVerifyOIDCState(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-oidc-state-secret")
	oidcStateSecretOnce = sync.Once{}
	oidcStateSecret = ""

	state, err := SignOIDCState(&OIDCStatePayload{
		Nonce:         "nonce-abc",
		RedirectURI:   "http://localhost:5173/login",
		PKCEChallenge: "test-challenge",
		IssuedAt:      time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("SignOIDCState: %v", err)
	}
	got, err := VerifyOIDCState(state)
	if err != nil {
		t.Fatalf("VerifyOIDCState: %v", err)
	}
	if got.Nonce != "nonce-abc" || got.RedirectURI != "http://localhost:5173/login" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestVerifyOIDCStateRejectsTamperedPayload(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-oidc-state-secret")
	oidcStateSecretOnce = sync.Once{}
	oidcStateSecret = ""

	state, err := SignOIDCState(&OIDCStatePayload{
		Nonce:         "nonce-abc",
		RedirectURI:   "http://localhost:5173/login",
		PKCEChallenge: "test-challenge",
		IssuedAt:      time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("SignOIDCState: %v", err)
	}
	parts := strings.Split(state, ".")
	tampered := parts[0] + ".AAAA"
	if _, err := VerifyOIDCState(tampered); err == nil {
		t.Fatal("expected tampered state to be rejected")
	}
}

func TestOIDCPKCEChallengeRFC7636(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	got, err := OIDCPKCEChallenge(verifier)
	if err != nil || got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("S256 challenge = %q, err = %v", got, err)
	}
	for _, invalid := range []string{"", strings.Repeat("a", 42), strings.Repeat("a", 129), strings.Repeat("a", 42) + "+", strings.Repeat("a", 42) + "中"} {
		if _, err := OIDCPKCEChallenge(invalid); err == nil {
			t.Fatal("accepted malformed verifier")
		}
	}
}

func TestVerifyOIDCStateRejectsLegacyOrExpiredState(t *testing.T) {
	for _, tc := range []struct {
		name      string
		challenge string
		issuedAt  time.Time
	}{
		{"legacy-no-pkce", "", time.Now()},
		{"expired", "challenge", time.Now().Add(-11 * time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Sign manually to exercise validation of an older, otherwise authentic state.
			raw, err := json.Marshal(&OIDCStatePayload{
				Nonce: "nonce", RedirectURI: "https://app.example/callback",
				PKCEChallenge: tc.challenge, IssuedAt: tc.issuedAt.Unix(),
			})
			if err != nil {
				t.Fatal(err)
			}
			mac := hmac.New(sha256.New, []byte(oidcStateSigningKey()))
			mac.Write(raw)
			state := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
			if _, err := VerifyOIDCState(state); err == nil {
				t.Fatal("accepted invalid state")
			}
		})
	}
}
