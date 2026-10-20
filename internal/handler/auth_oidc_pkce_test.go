package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

const testOIDCVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

func testOIDCState(t *testing.T, issuedAt time.Time) string {
	t.Helper()
	challenge, err := secutils.OIDCPKCEChallenge(testOIDCVerifier)
	if err != nil {
		t.Fatal(err)
	}
	state, err := secutils.SignOIDCState(&secutils.OIDCStatePayload{
		Nonce: "browser-nonce", RedirectURI: "https://app.example/api/v1/auth/oidc/callback",
		PKCEChallenge: challenge, IssuedAt: issuedAt.Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestOIDCEntrypointsKeepVerifierInSecureCookie(t *testing.T) {
	for _, path := range []string{"/auth/oidc/start", "/auth/oidc/url?redirect_uri=https%3A%2F%2Fapp.example%2Fcallback"} {
		t.Run(path, func(t *testing.T) {
			us := &stubOIDCStartUserService{
				getOIDCAuthorizationURL: func(context.Context, string) (*types.OIDCAuthURLResponse, error) {
					return &types.OIDCAuthURLResponse{
						Success: true, AuthorizationURL: "https://idp.example/authorize",
						State: testOIDCState(t, time.Now()), Nonce: "browser-nonce", CodeVerifier: testOIDCVerifier,
					}, nil
				},
			}
			h := NewAuthHandler(&config.Config{}, us, nil, nil, nil)
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(errorCapture())
			r.GET("/auth/oidc/start", h.OIDCStart)
			r.GET("/auth/oidc/url", h.GetOIDCAuthorizationURL)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("X-Forwarded-Proto", "https")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK && w.Code != http.StatusFound {
				t.Fatalf("unexpected status: %d", w.Code)
			}
			found := false
			for _, cookie := range w.Result().Cookies() {
				if cookie.Name != oidcPKCECookieName {
					continue
				}
				found = true
				if cookie.Value != testOIDCVerifier || !cookie.Secure || !cookie.HttpOnly ||
					cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge != 600 {
					t.Fatal("PKCE cookie lacks its required protection or lifetime")
				}
			}
			if !found || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing secret cookie or no-store")
			}
			if strings.Contains(w.Body.String(), testOIDCVerifier) || strings.Contains(w.Header().Get("Location"), testOIDCVerifier) {
				t.Fatal("verifier leaked outside HttpOnly cookie")
			}
		})
	}
}

func TestOIDCEntrypointsRejectPlaintextBeforeSendingSecretCookie(t *testing.T) {
	for _, path := range []string{"/auth/oidc/start", "/auth/oidc/url?redirect_uri=https%3A%2F%2Fapp.example%2Fcallback"} {
		t.Run(path, func(t *testing.T) {
			us := &stubOIDCStartUserService{
				getOIDCAuthorizationURL: func(context.Context, string) (*types.OIDCAuthURLResponse, error) {
					return &types.OIDCAuthURLResponse{
						Success: true, AuthorizationURL: "https://idp.example/authorize",
						Nonce: "browser-nonce", CodeVerifier: testOIDCVerifier,
					}, nil
				},
			}
			h := NewAuthHandler(&config.Config{}, us, nil, nil, nil)
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(errorCapture())
			r.GET("/auth/oidc/start", h.OIDCStart)
			r.GET("/auth/oidc/url", h.GetOIDCAuthorizationURL)
			req := httptest.NewRequest(http.MethodGet, "http://app.example"+path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "OIDC login requires HTTPS") {
				t.Fatalf("plaintext login was not explicitly rejected: %d", w.Code)
			}
			if len(w.Result().Cookies()) != 0 || w.Header().Get("Location") != "" ||
				strings.Contains(w.Body.String(), testOIDCVerifier) {
				t.Fatal("plaintext response exposed a cookie, verifier, or authorization URL")
			}
		})
	}
}

type stubPKCECallbackUserService struct {
	interfaces.UserService
	calls int
	fail  bool
	t     *testing.T
}

func (s *stubPKCECallbackUserService) LoginWithOIDC(_ context.Context, code, redirectURI, verifier string, _ types.TenantProvisioningMode) (*types.OIDCCallbackResponse, error) {
	s.calls++
	if code != "test-code" || redirectURI != "https://app.example/api/v1/auth/oidc/callback" || verifier != testOIDCVerifier {
		s.t.Error("callback passed unbound exchange values")
	}
	if s.fail {
		return nil, errors.New("provider unavailable")
	}
	return &types.OIDCCallbackResponse{Success: true, User: &types.User{ID: "test-user"}}, nil
}

func TestOIDCCallbackRequiresBoundVerifierAndCleansOnlyValidatedTransaction(t *testing.T) {
	valid := testOIDCState(t, time.Now())
	expired := testOIDCState(t, time.Now().Add(-11*time.Minute))
	for _, tc := range []struct {
		name, state, nonce, verifier, query, result string
		validated, exchange, fail                   bool
	}{
		{"success", valid, "browser-nonce", testOIDCVerifier, "code=test-code", "oidc_result=", true, true, false},
		{"missing-cookie", valid, "browser-nonce", "", "code=test-code", "oidc_error=invalid_state", false, false, false},
		{"other-transaction", valid, "browser-nonce", strings.Repeat("x", 43), "code=test-code", "oidc_error=invalid_state", false, false, false},
		{"bad-verifier", valid, "browser-nonce", "short", "code=test-code", "oidc_error=invalid_state", false, false, false},
		{"other-browser", valid, "different-nonce", testOIDCVerifier, "code=test-code", "oidc_error=invalid_state", false, false, false},
		{"tampered", valid + "x", "browser-nonce", testOIDCVerifier, "code=test-code", "oidc_error=invalid_state", false, false, false},
		{"expired", expired, "browser-nonce", testOIDCVerifier, "code=test-code", "oidc_error=invalid_state", false, false, false},
		{"missing-code", valid, "browser-nonce", testOIDCVerifier, "", "oidc_error=missing_code", true, false, false},
		{"provider-denied", valid, "browser-nonce", testOIDCVerifier, "error=access_denied", "oidc_error=access_denied", true, false, false},
		{"unbound-provider-error", valid, "", "", "error=access_denied", "oidc_error=invalid_state", false, false, false},
		{"provider-failure", valid, "browser-nonce", testOIDCVerifier, "code=test-code", "oidc_error=login_failed", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			us := &stubPKCECallbackUserService{t: t, fail: tc.fail}
			h := NewAuthHandler(&config.Config{}, us, nil, nil, nil)
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.GET("/callback", h.OIDCRedirectCallback)
			req := httptest.NewRequest(http.MethodGet, "/callback?state="+url.QueryEscape(tc.state)+"&"+tc.query, nil)
			if tc.nonce != "" {
				req.AddCookie(&http.Cookie{Name: oidcNonceCookieName, Value: tc.nonce})
			}
			if tc.verifier != "" {
				req.AddCookie(&http.Cookie{Name: oidcPKCECookieName, Value: tc.verifier})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Location"), tc.result) {
				t.Fatalf("unexpected callback result: status %d, location %s", w.Code, w.Header().Get("Location"))
			}
			if (us.calls == 1) != tc.exchange {
				t.Fatalf("exchange calls = %d", us.calls)
			}
			cookies := w.Result().Cookies()
			if tc.validated {
				if len(cookies) != 2 {
					t.Fatal("validated transaction cookies were not cleared")
				}
				for _, cookie := range cookies {
					if cookie.Value != "" || cookie.MaxAge >= 0 || (cookie.Name == oidcPKCECookieName && !cookie.Secure) {
						t.Fatal("transaction cookie was not securely expired")
					}
				}
			} else if len(cookies) != 0 {
				t.Fatal("unrelated callback cleared an active transaction")
			}
		})
	}
}
