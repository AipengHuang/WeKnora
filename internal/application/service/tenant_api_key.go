package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// apiKeyLastUsedMinInterval bounds how often we persist last_used_at per key.
// The UI only needs minute-level freshness; throttling avoids a DB write on
// every authenticated request under high QPS.
const apiKeyLastUsedMinInterval = time.Minute

type tenantAPIKeyService struct {
	repo          interfaces.TenantAPIKeyRepository
	lastUsedTouch sync.Map // key ID (uint64) -> time.Time of last persisted touch
}

func NewTenantAPIKeyService(repo interfaces.TenantAPIKeyRepository) interfaces.TenantAPIKeyService {
	return &tenantAPIKeyService{repo: repo}
}

func (s *tenantAPIKeyService) CreateAPIKey(
	ctx context.Context, req interfaces.TenantAPIKeyCreateRequest,
) (*interfaces.TenantAPIKeyCreateResult, error) {
	scopeType, err := types.ParseAPIKeyScopeType(req.ScopeType)
	if err != nil {
		return nil, fmt.Errorf("invalid API key scope: %w", err)
	}
	if scopeType == types.APIKeyScopeTenant && req.TenantID == 0 {
		return nil, errors.New("tenant_id is required")
	}
	if scopeType == types.APIKeyScopePlatform && req.FullAccess {
		return nil, errors.New("platform API keys require explicit capabilities")
	}
	capabilities, err := types.ParseAPIKeyCapabilities(types.StringArray(req.Capabilities))
	if err != nil {
		return nil, fmt.Errorf("invalid capabilities: %w", err)
	}
	if scopeType == types.APIKeyScopePlatform && len(capabilities) == 0 {
		return nil, errors.New("platform API keys require at least one capability")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	token, err := generateTenantAPIKeyToken()
	if err != nil {
		return nil, err
	}
	expiresAt := req.ExpiresAt
	if expiresAt != nil {
		utc := expiresAt.UTC()
		expiresAt = &utc
	}
	var tenantID *uint64
	if scopeType == types.APIKeyScopeTenant {
		tenantID = &req.TenantID
	}
	key := &types.TenantAPIKey{
		TenantID:         tenantID,
		ScopeType:        scopeType,
		Name:             name,
		KeyHash:          hashTenantAPIKey(token),
		APIKey:           token,
		FullAccess:       req.FullAccess,
		KnowledgeBaseIDs: normalizeAPIKeyIDs(req.KnowledgeBaseIDs),
		Capabilities:     capabilities,
		ExpiresAt:        expiresAt,
	}
	if key.FullAccess {
		key.KnowledgeBaseIDs = nil
		key.Capabilities = nil
	}
	if err := s.repo.CreateAPIKey(ctx, key); err != nil {
		return nil, err
	}
	// 仓储加密钩子可能改写模型字段；创建边界只恢复本次一次性令牌。
	key.APIKey = token
	return &interfaces.TenantAPIKeyCreateResult{APIKey: key, Token: token}, nil
}

func (s *tenantAPIKeyService) AuthenticateAPIKey(ctx context.Context, token string) (*types.TenantAPIKey, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, apprepo.ErrTenantAPIKeyNotFound
	}
	key, err := s.repo.GetAPIKeyByHash(ctx, hashTenantAPIKey(token))
	if err != nil {
		return nil, err
	}
	if err := validateStoredAPIKeyProtocol(key); err != nil {
		return nil, err
	}
	if key.RevokedAt != nil {
		return nil, apprepo.ErrTenantAPIKeyNotFound
	}
	if key.ExpiresAt != nil && time.Now().UTC().After(key.ExpiresAt.UTC()) {
		return nil, apprepo.ErrTenantAPIKeyNotFound
	}
	s.touchAPIKeyLastUsedAsync(key.ID)
	return key, nil
}

// touchAPIKeyLastUsedAsync persists last_used_at at most once per key per
// apiKeyLastUsedMinInterval. The write runs in a detached goroutine so auth
// latency is not tied to an UPDATE on the hot path.
func (s *tenantAPIKeyService) touchAPIKeyLastUsedAsync(keyID uint64) {
	now := time.Now().UTC()
	if v, ok := s.lastUsedTouch.Load(keyID); ok {
		if now.Sub(v.(time.Time)) < apiKeyLastUsedMinInterval {
			return
		}
	}
	s.lastUsedTouch.Store(keyID, now)
	go func(id uint64, at time.Time) {
		if err := s.repo.UpdateAPIKeyLastUsed(context.Background(), id, at); err != nil {
			logger.Warnf(context.Background(),
				"failed to update tenant api key last_used_at (id=%d): %v", id, err)
			s.lastUsedTouch.Delete(id)
		}
	}(keyID, now)
}

func (s *tenantAPIKeyService) ListAPIKeys(ctx context.Context, tenantID uint64) ([]*types.TenantAPIKey, error) {
	keys, err := s.repo.ListAPIKeys(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := prepareAPIKeysForNonCreateResponse(keys); err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *tenantAPIKeyService) ListPlatformAPIKeys(ctx context.Context) ([]*types.TenantAPIKey, error) {
	keys, err := s.repo.ListPlatformAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	if err := prepareAPIKeysForNonCreateResponse(keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// UpdateAPIKey 按创建接口的相同语义更新租户 API Key 配置。
// scoped Key 需要至少一个能力；full-access Key 会清空细粒度能力和知识库范围。
func (s *tenantAPIKeyService) UpdateAPIKey(
	ctx context.Context, req interfaces.TenantAPIKeyUpdateRequest,
) (*types.TenantAPIKey, error) {
	if req.TenantID == 0 {
		return nil, errors.New("tenant_id is required")
	}
	if req.APIKeyID == 0 {
		return nil, errors.New("api_key_id is required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	capabilities, err := types.ParseAPIKeyCapabilities(types.StringArray(req.Capabilities))
	if err != nil {
		return nil, fmt.Errorf("invalid capabilities: %w", err)
	}
	if !req.FullAccess && len(capabilities) == 0 {
		return nil, errors.New("capabilities are required for scoped API keys")
	}
	expiresAt := req.ExpiresAt
	if expiresAt != nil {
		utc := expiresAt.UTC()
		expiresAt = &utc
	}
	key := &types.TenantAPIKey{
		Name:             name,
		FullAccess:       req.FullAccess,
		KnowledgeBaseIDs: normalizeAPIKeyIDs(req.KnowledgeBaseIDs),
		Capabilities:     capabilities,
		ExpiresAt:        expiresAt,
	}
	if key.FullAccess {
		key.KnowledgeBaseIDs = nil
		key.Capabilities = nil
	}
	updated, err := s.repo.UpdateAPIKey(ctx, req.TenantID, req.APIKeyID, key)
	if err != nil {
		return nil, err
	}
	if updated != nil {
		updated.APIKey = ""
	}
	return updated, nil
}

// prepareAPIKeysForNonCreateResponse 校验存储协议并清除非创建响应中的可复用凭据。
func prepareAPIKeysForNonCreateResponse(keys []*types.TenantAPIKey) error {
	for _, key := range keys {
		if err := validateStoredAPIKeyProtocol(key); err != nil {
			return err
		}
		key.APIKey = ""
	}
	return nil
}

// validateStoredAPIKeyProtocol 校验仓储结果使用精确作用域和能力协议。
func validateStoredAPIKeyProtocol(key *types.TenantAPIKey) error {
	if key == nil {
		return errors.New("stored API key is nil")
	}
	scopeType, err := types.ParseAPIKeyScopeType(key.ScopeType)
	if err != nil {
		return fmt.Errorf("invalid stored API key scope: %w", err)
	}
	capabilities, err := types.ParseAPIKeyCapabilities(key.Capabilities)
	if err != nil {
		return fmt.Errorf("invalid stored API key capabilities: %w", err)
	}
	switch scopeType {
	case types.APIKeyScopePlatform:
		if key.TenantID != nil || key.FullAccess || len(capabilities) == 0 {
			return errors.New("invalid stored platform API key scope")
		}
	case types.APIKeyScopeTenant:
		if key.TenantID == nil || *key.TenantID == 0 {
			return errors.New("invalid stored tenant API key scope")
		}
	}
	return nil
}

func (s *tenantAPIKeyService) RevokeAPIKey(ctx context.Context, tenantID uint64, id uint64) error {
	return s.repo.RevokeAPIKey(ctx, tenantID, id)
}

func (s *tenantAPIKeyService) RevokePlatformAPIKey(ctx context.Context, id uint64) error {
	return s.repo.RevokePlatformAPIKey(ctx, id)
}

func (s *tenantAPIKeyService) BackfillMissingKeyHashes(ctx context.Context) (int, error) {
	has, err := s.repo.HasKeysWithPlaceholderHash(ctx)
	if err != nil {
		return 0, err
	}
	if !has {
		return 0, nil
	}
	keys, err := s.repo.ListKeysWithPlaceholderHash(ctx)
	if err != nil {
		return 0, err
	}
	backfilled := 0
	for _, key := range keys {
		if key == nil || strings.TrimSpace(key.APIKey) == "" {
			continue
		}
		hash := hashTenantAPIKey(key.APIKey)
		if key.KeyHash == hash {
			continue
		}
		if err := s.repo.UpdateAPIKeyHash(ctx, key.ID, hash); err != nil {
			return backfilled, err
		}
		backfilled++
	}
	return backfilled, nil
}

func generateTenantAPIKeyToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "sk-" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashTenantAPIKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func normalizeAPIKeyIDs(in []string) types.StringArray {
	out := types.StringArray{}
	seen := map[string]struct{}{}
	for _, id := range in {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
