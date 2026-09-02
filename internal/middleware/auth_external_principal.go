package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultExternalUserIDHeader    = "X-External-User-ID"
	defaultExternalUserTokenHeader = "X-External-User-Token"
	maxExternalUserIDLen           = 128
	maxExternalUserTokenTTL        = 24 * time.Hour
)

var (
	errMissingDirectHeader      = errors.New("missing external user id header")
	errInvalidExternalUserID    = errors.New("invalid external user id")
	errInvalidExternalUserToken = errors.New("invalid external user token")
)

func resolveAPIPrincipal(ctx context.Context, tenant *types.Tenant, header http.Header) (types.Principal, error) {
	tenantID := uint64(0)
	if tenant != nil {
		tenantID = tenant.ID
	}
	fallback := types.Principal{
		Type: types.PrincipalAPITenant,
		ID:   strconv.FormatUint(tenantID, 10),
	}
	if tenant == nil || tenantID == 0 {
		return fallback, nil
	}
	cfg := tenant.APIPrincipalConfig
	if cfg == nil || cfg.Mode == "" || cfg.Mode == types.APIPrincipalModeTenant {
		return fallback, nil
	}
	switch cfg.Mode {
	case types.APIPrincipalModeDirect:
		externalUserID := strings.TrimSpace(header.Get(defaultExternalUserIDHeader))
		if externalUserID == "" {
			if cfg.RequireDirectHeader {
				return types.Principal{}, errMissingDirectHeader
			}
			return fallback, nil
		}
		if err := validateExternalUserID(externalUserID); err != nil {
			return types.Principal{}, fmt.Errorf("%w: %v", errInvalidExternalUserID, err)
		}
		return types.Principal{
			Type: types.PrincipalAPIExternalUser,
			ID:   strconv.FormatUint(tenantID, 10) + ":" + externalUserID,
		}, nil
	case types.APIPrincipalModeSignedToken:
		externalUserID, err := verifyExternalUserJWT(header.Get(defaultExternalUserTokenHeader), tenantID, cfg.HMACSecret)
		if err != nil || externalUserID == "" {
			logger.Warnf(ctx, "invalid external user token for tenant=%d: %v", tenantID, err)
			return types.Principal{}, fmt.Errorf("%w: %w", errInvalidExternalUserToken, err)
		}
		if err := validateExternalUserID(externalUserID); err != nil {
			return types.Principal{}, fmt.Errorf("%w: %v", errInvalidExternalUserID, err)
		}
		return types.Principal{
			Type: types.PrincipalAPIExternalUser,
			ID:   strconv.FormatUint(tenantID, 10) + ":" + externalUserID,
		}, nil
	default:
		return fallback, nil
	}
}

func verifyExternalUserJWT(tokenString string, tenantID uint64, secret string) (string, error) {
	tokenString = strings.TrimSpace(tokenString)
	secret = strings.TrimSpace(secret)
	if tokenString == "" {
		return "", errors.New("missing external user token")
	}
	if secret == "" {
		return "", errors.New("external user token secret is not configured")
	}
	claims := jwt.MapClaims{}
	parser := jwt.NewParser(
		jwt.WithAudience("weknora"),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	token, err := parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	if token == nil || !token.Valid {
		return "", errors.New("invalid external user token")
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return "", errors.New("missing expiration")
	}
	if time.Until(exp.Time) > maxExternalUserTokenTTL {
		return "", fmt.Errorf("token lifetime exceeds %s", maxExternalUserTokenTTL)
	}
	if nbf, nbfErr := claims.GetNotBefore(); nbfErr == nil && nbf != nil && time.Now().Before(nbf.Time) {
		return "", errors.New("token not yet valid")
	}
	if got := principalTenantIDFromClaims(claims); got != tenantID {
		return "", fmt.Errorf("workspace mismatch: got %d want %d", got, tenantID)
	}
	sub, _ := claims["sub"].(string)
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return "", errors.New("missing subject")
	}
	return sub, nil
}

func validateExternalUserID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("empty external user id")
	}
	if len(id) > maxExternalUserIDLen {
		return fmt.Errorf("external user id too long (max %d)", maxExternalUserIDLen)
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return errors.New("external user id contains invalid characters")
		}
	}
	return nil
}

func apiPrincipalAuthErrorMessage(err error) string {
	switch {
	case errors.Is(err, errMissingDirectHeader):
		return "Unauthorized: missing external user id header"
	case errors.Is(err, errInvalidExternalUserID):
		return "Unauthorized: invalid external user id"
	case errors.Is(err, errInvalidExternalUserToken):
		return "Unauthorized: invalid external user token"
	default:
		return "Unauthorized: invalid external user token"
	}
}

func principalTenantIDFromClaims(claims jwt.MapClaims) uint64 {
	v, ok := claims["tenant_id"]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		if t <= 0 {
			return 0
		}
		return uint64(t)
	case int64:
		if t <= 0 {
			return 0
		}
		return uint64(t)
	case uint64:
		return t
	case json.Number:
		n, err := strconv.ParseUint(t.String(), 10, 64)
		if err != nil {
			return 0
		}
		return n
	case string:
		n, err := strconv.ParseUint(strings.TrimSpace(t), 10, 64)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

// resolveTenantRole determines the caller's TenantRole inside targetTenantID.
//
// Order of resolution:
//  1. Active TenantMember row → return that role.
//  2. Cross-tenant superuser switch (X-Tenant-ID with CanAccessAllTenants=true)
//     → grant Admin in the target tenant. Org admins are intentionally not
//     promoted to Owner; tenant deletion / API-key rotation should always
//     stay with a real Owner inside the target tenant. Cross-tenant access
//     is also never allowed to trigger the orphan-tenant auto-promotion
//     below — a superuser only visits, never claims ownership.
//  3. No membership but the tenant currently has zero active members AND
//     the caller is authenticating into their own home tenant (i.e.
//     targetTenantID == user.TenantID and this is not a cross-tenant
//     switch). This is the API-key-only orphan-tenant self-heal path:
//     the registrant becomes Owner of the tenant their own user record
//     points to. Any other path (cross-tenant switch, JWT minted for a
//     foreign tenant, etc.) is intentionally excluded to avoid silent
//     ownership grabs.
//  4. Otherwise → return ok=false. Caller decides:
//     - When EnableRBAC=true (or cfg unavailable): treat as 403.
//     - When EnableRBAC=false: fail open with Admin so existing deployments
//     don't break in the rollout window where memberships might lag user
//     records.
//
// The boolean second return value reports whether enforcement should reject
// the request. It is true whenever a usable role was found OR fail-open
// applies; false only when we want callers to abort with 403.
