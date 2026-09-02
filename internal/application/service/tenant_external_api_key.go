package service

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

func (s *tenantAPIKeyService) PutExternalTenantAPIKey(
	ctx context.Context,
	req interfaces.ExternalTenantAPIKeyPutRequest,
) (*interfaces.ExternalTenantAPIKeyPutResult, error) {
	if req.Name == "" || req.Name != strings.TrimSpace(req.Name) {
		return nil, types.ErrExternalTenantCredentialProtocol
	}
	capabilityValues := make(types.StringArray, len(req.Capabilities))
	for index, capability := range req.Capabilities {
		capabilityValues[index] = string(capability)
	}
	capabilities, err := types.ParseAPIKeyCapabilities(capabilityValues)
	if err != nil || len(capabilities) == 0 {
		return nil, types.ErrExternalTenantCredentialProtocol
	}
	token, err := generateTenantAPIKeyToken()
	if err != nil {
		return nil, err
	}
	key := &types.TenantAPIKey{
		ScopeType:        types.APIKeyScopeTenant,
		Name:             req.Name,
		KeyHash:          hashTenantAPIKey(token),
		APIKey:           token,
		KnowledgeBaseIDs: make(types.StringArray, 0),
		Capabilities:     capabilities,
	}
	stored, created, err := s.repo.PutExternalTenantAPIKey(
		ctx, req.TenantRef, req.CredentialRef, key,
	)
	if err != nil {
		return nil, err
	}
	if !matchesExternalTenantAPIKeyProtocol(stored, req, capabilities) {
		return nil, types.ErrExternalTenantCredentialConflict
	}
	return &interfaces.ExternalTenantAPIKeyPutResult{
		APIKey: stored, Token: stored.APIKey, Created: created,
	}, nil
}

func matchesExternalTenantAPIKeyProtocol(
	stored *types.TenantAPIKey,
	req interfaces.ExternalTenantAPIKeyPutRequest,
	capabilities types.StringArray,
) bool {
	if stored == nil || stored.TenantID == nil || *stored.TenantID == 0 ||
		stored.ExternalRef == nil || *stored.ExternalRef != req.CredentialRef.String() ||
		stored.ScopeType != types.APIKeyScopeTenant || stored.Name != req.Name ||
		stored.FullAccess || len(stored.KnowledgeBaseIDs) != 0 ||
		stored.ExpiresAt != nil || stored.RevokedAt != nil ||
		stored.APIKey == "" || stored.APIKey != strings.TrimSpace(stored.APIKey) ||
		stored.KeyHash != hashTenantAPIKey(stored.APIKey) ||
		len(stored.Capabilities) != len(capabilities) {
		return false
	}
	for index, capability := range capabilities {
		if stored.Capabilities[index] != capability {
			return false
		}
	}
	return true
}
