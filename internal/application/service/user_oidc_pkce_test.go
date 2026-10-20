package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

func TestOIDCAuthorizationURLBindsFreshS256Verifier(t *testing.T) {
	withOIDCSSRFWhitelist(t, "127.0.0.1")
	svc := &userService{config: &config.Config{OIDCAuth: &config.OIDCAuthConfig{
		Enable: true, ClientID: "client", ClientSecret: "secret",
		AuthorizationEndpoint: "https://127.0.0.1/authorize",
		TokenEndpoint:         "https://127.0.0.1/token", JwksURI: "https://127.0.0.1/keys",
		IssuerURL: "https://127.0.0.1", Scopes: []string{"openid"},
	}}}
	seen := map[string]bool{}
	for range 2 {
		resp, err := svc.GetOIDCAuthorizationURL(context.Background(), "https://app.example/callback")
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.CodeVerifier) != 43 || seen[resp.CodeVerifier] {
			t.Fatal("verifier is not fresh 256-bit base64url")
		}
		seen[resp.CodeVerifier] = true
		u, err := url.Parse(resp.AuthorizationURL)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		challenge, err := secutils.OIDCPKCEChallenge(resp.CodeVerifier)
		if err != nil || q.Get("code_challenge") != challenge || q.Get("code_challenge_method") != "S256" {
			t.Fatal("authorization request is missing S256 binding")
		}
		state, err := secutils.VerifyOIDCState(q.Get("state"))
		if err != nil || state.PKCEChallenge != challenge {
			t.Fatal("signed state is not bound to the verifier")
		}
		rawState, err := base64.RawURLEncoding.DecodeString(strings.Split(resp.State, ".")[0])
		if err != nil {
			t.Fatal(err)
		}
		publicJSON, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		for _, public := range []string{resp.AuthorizationURL, string(rawState), string(publicJSON)} {
			if strings.Contains(public, resp.CodeVerifier) || strings.Contains(public, "code_verifier") {
				t.Fatal("verifier leaked in public authorization data")
			}
		}
	}
}

func TestOIDCTokenExchangeRequiresVerifierAndNeverDowngrades(t *testing.T) {
	withOIDCSSRFWhitelist(t, "127.0.0.1")
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	calls := 0
	reject := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPost || r.PostForm.Get("code_verifier") != verifier ||
			r.PostForm.Get("client_secret") != "secret" ||
			r.PostForm.Get("code") != "code" || r.PostForm.Get("grant_type") != "authorization_code" {
			t.Error("token exchange omitted PKCE or existing client authentication")
		}
		if reject {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid_grant " + verifier))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": "signed-token"})
	}))
	defer server.Close()
	svc := &userService{}
	cfg := &config.OIDCAuthConfig{TokenEndpoint: server.URL, ClientID: "client", ClientSecret: "secret"}
	for _, invalid := range []string{"", "short", strings.Repeat("+", 43)} {
		if _, err := svc.exchangeOIDCCode(context.Background(), cfg, "code", "https://app.example/callback", invalid); err == nil {
			t.Fatal("accepted missing or invalid verifier")
		}
	}
	if calls != 0 {
		t.Fatal("invalid verifier reached token endpoint")
	}
	if _, err := svc.exchangeOIDCCode(context.Background(), cfg, "code", "https://app.example/callback", verifier); err != nil {
		t.Fatal(err)
	}
	reject = true
	_, err := svc.exchangeOIDCCode(context.Background(), cfg, "code", "https://app.example/callback", verifier)
	if err == nil || strings.Contains(err.Error(), verifier) {
		t.Fatal("provider failure was ignored or leaked verifier")
	}
	if calls != 2 {
		t.Fatal("token exchange retried after failure")
	}
}
