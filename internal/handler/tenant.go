package handler

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// TenantHandler implements HTTP request handlers for tenant management
// Provides functionality for creating, retrieving, updating, and deleting tenants
// through the REST API endpoints
type TenantHandler struct {
	service       interfaces.TenantService
	apiKeyService interfaces.TenantAPIKeyService
	userService   interfaces.UserService
	memberService interfaces.TenantMemberService
	kbService     interfaces.KnowledgeBaseService
	config        *config.Config
	// systemSettingSvc resolves runtime tenant policies and limits.
	// Reading goes DB > ENV >
	// in-code default, so a SystemAdmin's UI override applies on the
	// very next CreateTenant call.
	systemSettingSvc interfaces.SystemSettingService
}

// NewTenantHandler creates a new tenant handler instance with the provided service
// Parameters:
//   - service: An implementation of the TenantService interface for business logic
//   - userService: An implementation of the UserService interface for user operations
//   - memberService: An implementation of TenantMemberService used to bootstrap
//     the creator as Owner of the tenant they just created (self-service create).
//   - config: Application configuration
//
// # Returns a pointer to the newly created TenantHandler
//
// Note on RBAC: cross-tenant gating (CanAccessAllTenants /
// EnableCrossTenantAccess) and per-tenant path matching (URL :id ==
// active tenant) used to live in `authorizeTenantAccess` and the if
// blocks at the top of ListAllTenants / SearchTenants. Both moved to
// `middleware/access.go` (RequireCrossTenantAccess /
// RequirePathTenantMatch) and are wired in `router.go` so the handler
// stays focused on business logic.
func NewTenantHandler(
	service interfaces.TenantService,
	apiKeyService interfaces.TenantAPIKeyService,
	userService interfaces.UserService,
	memberService interfaces.TenantMemberService,
	kbService interfaces.KnowledgeBaseService,
	config *config.Config,
	systemSettingSvc interfaces.SystemSettingService,
) *TenantHandler {
	return &TenantHandler{
		service:          service,
		apiKeyService:    apiKeyService,
		userService:      userService,
		memberService:    memberService,
		kbService:        kbService,
		config:           config,
		systemSettingSvc: systemSettingSvc,
	}
}

// createTenantRequest is the JSON body for POST /tenants. Only fields a
// regular authenticated user is allowed to set are accepted; everything
// else (api_key, status, storage_quota, retriever_engines, etc.) is
// generated server-side by TenantService.CreateTenant so a normal user
// can't bypass quotas or self-suspend a workspace at create time.
//
// Cross-tenant superusers historically posted the full Tenant struct to
// this endpoint. We keep that path working by binding into the same
// types.Tenant when CanAccessAllTenants is true (see CreateTenant
// below), but the recommended shape going forward is name+description.
type createTenantRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=128"`
	Description string `json:"description" binding:"max=512"`
}

// updateTenantRequest is the JSON body for PUT /tenants/:id. Only the
// fields an Owner is permitted to mutate via the public API are bound;
// everything else (storage_quota, status, business, api_key, agent /
// retrieval / storage configs, ...) is intentionally NOT writable here
// — those go through dedicated endpoints (PUT /tenants/kv/:key, ...)
// that have their own validation.
//
// Pointers so we can distinguish "not sent" from "explicit empty
// string"; when nil we leave the existing column untouched.
type updateTenantRequest struct {
	Name        *string `json:"name"        binding:"omitempty,min=1,max=128"`
	Description *string `json:"description" binding:"omitempty,max=512"`
}

type apiPrincipalConfigRequest struct {
	Mode                  types.APIPrincipalMode `json:"mode"`
	DirectHeaderName      string                 `json:"direct_header_name"`
	SignedTokenHeaderName string                 `json:"signed_token_header_name"`
	RequireDirectHeader   bool                   `json:"require_direct_header"`
	HMACSecret            *string                `json:"hmac_secret"`
}

type apiPrincipalConfigResponse struct {
	Mode                  types.APIPrincipalMode `json:"mode"`
	DirectHeaderName      string                 `json:"direct_header_name"`
	SignedTokenHeaderName string                 `json:"signed_token_header_name"`
	RequireDirectHeader   bool                   `json:"require_direct_header"`
	// HasHMACSecret reports whether a signing secret is configured. The
	// plaintext secret is NEVER returned — clients only learn presence.
	HasHMACSecret bool `json:"has_hmac_secret"`
}

type apiPrincipalTestTokenRequest struct {
	ExternalUserID   string `json:"external_user_id"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

type apiPrincipalTestTokenResponse struct {
	Token            string `json:"token"`
	HeaderName       string `json:"header_name"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
	ExpiresAtUnix    int64  `json:"expires_at_unix"`
	ExternalUserID   string `json:"external_user_id"`
}

type tenantAPIKeyCreateRequest struct {
	Name             string   `json:"name"`
	FullAccess       bool     `json:"full_access"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids"`
	Capabilities     []string `json:"capabilities"`
	ExpiresAt        *int64   `json:"expires_at_unix"`
}

// tenantAPIKeyUpdateRequest 修改已创建 API Key 的配置，字段语义与创建接口一致。
type tenantAPIKeyUpdateRequest struct {
	Name             string   `json:"name"`
	FullAccess       bool     `json:"full_access"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids"`
	Capabilities     []string `json:"capabilities"`
	ExpiresAt        *int64   `json:"expires_at_unix"`
}

type tenantAPIKeyResponse struct {
	ID               uint64                `json:"id"`
	ScopeType        types.APIKeyScopeType `json:"scope_type"`
	Name             string                `json:"name"`
	APIKey           string                `json:"api_key"`
	FullAccess       bool                  `json:"full_access"`
	KnowledgeBaseIDs types.StringArray     `json:"knowledge_base_ids"`
	Capabilities     types.StringArray     `json:"capabilities"`
	LastUsedAt       *time.Time            `json:"last_used_at,omitempty"`
	ExpiresAt        *time.Time            `json:"expires_at,omitempty"`
	CreatedAt        time.Time             `json:"created_at"`
}

type tenantAPIKeyCreateResponse struct {
	tenantAPIKeyResponse
	Token string `json:"token"`
}

const (
	defaultAPIPrincipalDirectHeader  = "X-External-User-ID"
	defaultAPIPrincipalTokenHeader   = "X-External-User-Token"
	defaultAPIPrincipalTestTokenTTL  = 15 * time.Minute
	maxAPIPrincipalTestTokenTTL      = time.Hour
	maxAPIPrincipalExternalUserIDLen = 128
)

// defaultMaxOwnedTenantsPerUser is the cap applied when
// config.Tenant.MaxOwnedPerUser is left at zero. Picked to comfortably
// cover legitimate "personal + a couple of side-projects" use while
// blunting drive-by abuse against POST /tenants (see CreateTenant).
const defaultMaxOwnedTenantsPerUser = 10

// resolveMaxOwnedTenantsPerUser returns the current cap, walking the
// 3-tier resolver: system_settings DB row > WEKNORA_TENANT_MAX_OWNED_PER_USER
// env > config.Tenant.MaxOwnedPerUser (yaml) > defaultMaxOwnedTenantsPerUser.
// We pre-compute the cfg-derived fallback so the SystemSettingService
// receives a single int64 default — its 3-tier resolver layers DB and
// env on top of that.
func (h *TenantHandler) resolveMaxOwnedTenantsPerUser(ctx context.Context) int {
	fallback := int64(defaultMaxOwnedTenantsPerUser)
	if h.config != nil && h.config.Tenant != nil && h.config.Tenant.MaxOwnedPerUser != 0 {
		fallback = int64(h.config.Tenant.MaxOwnedPerUser)
	}
	return int(h.systemSettingSvc.GetInt(
		ctx,
		"tenant.max_owned_per_user",
		"WEKNORA_TENANT_MAX_OWNED_PER_USER",
		fallback,
	))
}
