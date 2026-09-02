package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func signedExternalUserToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func TestResolveAPIPrincipalExplicitTenantMode(t *testing.T) {
	p, err := resolveAPIPrincipal(context.Background(), &types.Tenant{
		ID:                 7,
		APIPrincipalConfig: &types.APIPrincipalConfig{Mode: types.APIPrincipalModeTenant},
	}, http.Header{})
	if err != nil {
		t.Fatalf("resolveAPIPrincipal error = %v", err)
	}

	if p.Type != types.PrincipalAPITenant || p.ID != "7" {
		t.Fatalf("principal = %#v", p)
	}
}

func TestResolveAPIPrincipalDirectHeader(t *testing.T) {
	header := http.Header{}
	header.Set("X-External-User-ID", "external-u1")

	p, err := resolveAPIPrincipal(context.Background(), &types.Tenant{
		ID: 7,
		APIPrincipalConfig: &types.APIPrincipalConfig{
			Mode: types.APIPrincipalModeDirect,
		},
	}, header)
	if err != nil {
		t.Fatalf("resolveAPIPrincipal error = %v", err)
	}

	if p.Type != types.PrincipalAPIExternalUser || p.ID != "7:external-u1" {
		t.Fatalf("principal = %#v", p)
	}
}

func TestResolveAPIPrincipalSignedToken(t *testing.T) {
	secret := "test-secret"
	header := http.Header{}
	header.Set("X-External-User-Token", signedExternalUserToken(t, secret, jwt.MapClaims{
		"sub":       "external-u1",
		"tenant_id": float64(7),
		"aud":       "weknora",
		"exp":       time.Now().Add(time.Minute).Unix(),
	}))

	p, err := resolveAPIPrincipal(context.Background(), &types.Tenant{
		ID: 7,
		APIPrincipalConfig: &types.APIPrincipalConfig{
			Mode:       types.APIPrincipalModeSignedToken,
			HMACSecret: secret,
		},
	}, header)
	if err != nil {
		t.Fatalf("resolveAPIPrincipal error = %v", err)
	}

	if p.Type != types.PrincipalAPIExternalUser || p.ID != "7:external-u1" {
		t.Fatalf("principal = %#v", p)
	}
}

func TestResolveAPIPrincipalSignedTokenRejectsWrongTenant(t *testing.T) {
	secret := "test-secret"
	header := http.Header{}
	header.Set("X-External-User-Token", signedExternalUserToken(t, secret, jwt.MapClaims{
		"sub":       "external-u1",
		"tenant_id": float64(8),
		"aud":       "weknora",
		"exp":       time.Now().Add(time.Minute).Unix(),
	}))

	p, err := resolveAPIPrincipal(context.Background(), &types.Tenant{
		ID: 7,
		APIPrincipalConfig: &types.APIPrincipalConfig{
			Mode:       types.APIPrincipalModeSignedToken,
			HMACSecret: secret,
		},
	}, header)
	if err == nil {
		t.Fatalf("resolveAPIPrincipal error = nil, want error")
	}
	_ = p
}

func TestResolveAPIPrincipalSignedTokenRejectsExpired(t *testing.T) {
	secret := "test-secret"
	header := http.Header{}
	header.Set("X-External-User-Token", signedExternalUserToken(t, secret, jwt.MapClaims{
		"sub":       "external-u1",
		"tenant_id": float64(7),
		"aud":       "weknora",
		"exp":       time.Now().Add(-time.Minute).Unix(),
	}))

	p, err := resolveAPIPrincipal(context.Background(), &types.Tenant{
		ID: 7,
		APIPrincipalConfig: &types.APIPrincipalConfig{
			Mode:       types.APIPrincipalModeSignedToken,
			HMACSecret: secret,
		},
	}, header)
	if err == nil {
		t.Fatalf("resolveAPIPrincipal error = nil, want error")
	}
	_ = p
}

func TestResolveAPIPrincipalSignedTokenRequiresConfiguredSecret(t *testing.T) {
	header := http.Header{}
	header.Set("X-External-User-Token", signedExternalUserToken(t, "test-secret", jwt.MapClaims{
		"sub":       "external-u1",
		"tenant_id": float64(7),
		"aud":       "weknora",
		"exp":       time.Now().Add(time.Minute).Unix(),
	}))
	_, err := resolveAPIPrincipal(context.Background(), &types.Tenant{
		ID:                 7,
		APIPrincipalConfig: &types.APIPrincipalConfig{Mode: types.APIPrincipalModeSignedToken},
	}, header)
	if !errors.Is(err, errInvalidExternalUserToken) {
		t.Fatalf("resolveAPIPrincipal error = %v, want errInvalidExternalUserToken", err)
	}
}

func TestResolveAPIPrincipalDirectHeaderRejectsInvalidUserID(t *testing.T) {
	header := http.Header{}
	header.Set("X-External-User-ID", strings.Repeat("a", maxExternalUserIDLen+1))

	_, err := resolveAPIPrincipal(context.Background(), &types.Tenant{
		ID: 7,
		APIPrincipalConfig: &types.APIPrincipalConfig{
			Mode: types.APIPrincipalModeDirect,
		},
	}, header)
	if !errors.Is(err, errInvalidExternalUserID) {
		t.Fatalf("resolveAPIPrincipal error = %v, want errInvalidExternalUserID", err)
	}
}

func TestResolveAPIPrincipalSignedTokenRejectsLongLifetime(t *testing.T) {
	secret := "test-secret"
	header := http.Header{}
	header.Set("X-External-User-Token", signedExternalUserToken(t, secret, jwt.MapClaims{
		"sub":       "external-u1",
		"tenant_id": float64(7),
		"aud":       "weknora",
		"exp":       time.Now().Add(48 * time.Hour).Unix(),
	}))

	_, err := resolveAPIPrincipal(context.Background(), &types.Tenant{
		ID: 7,
		APIPrincipalConfig: &types.APIPrincipalConfig{
			Mode:       types.APIPrincipalModeSignedToken,
			HMACSecret: secret,
		},
	}, header)
	if err == nil {
		t.Fatalf("resolveAPIPrincipal error = nil, want error")
	}
}

// TestAuthRejectsTenantKeyWithoutPrincipalConfig 验证认证后的严格主体校验不会接受降级主体。
func TestAuthRejectsTenantKeyWithoutPrincipalConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID := uint64(7)
	engine := gin.New()
	engine.Use(Auth(
		&fakeTenantService{tenant: &types.Tenant{ID: tenantID}},
		&apiPrincipalUserService{},
		nil,
		&apiPrincipalKeyService{key: &types.TenantAPIKey{
			ID:        9,
			TenantID:  &tenantID,
			ScopeType: types.APIKeyScopeTenant,
		}},
		nil,
	))
	engine.Use(StrictAPIPrincipal())
	reached := false
	engine.GET("/api/v1/probe", func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/probe", nil)
	request.Header.Set("X-API-Key", "test-runtime-key")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || reached {
		t.Fatalf("status = %d, reached=%v, want 401 and false", response.Code, reached)
	}
}

// TestAuthRejectsDirectModeWithoutHeader 验证旧的可选标志不能绕过严格主体协议。
func TestAuthRejectsDirectModeWithoutHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID := uint64(7)
	engine := gin.New()
	engine.Use(Auth(
		&fakeTenantService{tenant: &types.Tenant{ID: tenantID, APIPrincipalConfig: &types.APIPrincipalConfig{
			Mode:                types.APIPrincipalModeDirect,
			RequireDirectHeader: false,
		}}},
		&apiPrincipalUserService{},
		nil,
		&apiPrincipalKeyService{key: &types.TenantAPIKey{
			ID: 9, TenantID: &tenantID, ScopeType: types.APIKeyScopeTenant,
		}},
		nil,
	))
	engine.Use(StrictAPIPrincipal())
	reached := false
	engine.GET("/api/v1/probe", func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/probe", nil)
	request.Header.Set("X-API-Key", "test-runtime-key")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || reached {
		t.Fatalf("status = %d, reached=%v, want 401 and false", response.Code, reached)
	}
}

type apiPrincipalUserService struct {
	interfaces.UserService
}

func (*apiPrincipalUserService) GetUserByTenantID(context.Context, uint64) (*types.User, error) {
	return &types.User{ID: "tenant-owner", IsActive: true}, nil
}

type apiPrincipalKeyService struct {
	interfaces.TenantAPIKeyService
	key *types.TenantAPIKey
}

func (s *apiPrincipalKeyService) AuthenticateAPIKey(context.Context, string) (*types.TenantAPIKey, error) {
	return s.key, nil
}
