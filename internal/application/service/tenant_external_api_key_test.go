package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

func TestPutExternalTenantAPIKeyReturnsStoredCredentialProtocol(t *testing.T) {
	tenantRef, _ := types.ParseExternalTenantRef("7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7")
	credentialRef, _ := types.ParseExternalTenantCredentialRef("a8af976f-47bd-5a9f-a270-4e92361e9a9d")
	repository := &externalTenantAPIKeyRepositoryStub{created: false}
	service := NewTenantAPIKeyService(repository)

	result, err := service.PutExternalTenantAPIKey(
		context.Background(),
		interfaces.ExternalTenantAPIKeyPutRequest{
			TenantRef:     tenantRef,
			CredentialRef: credentialRef,
			Name:          "adax-web-runtime-v1",
			Capabilities: []types.APIKeyCapability{
				types.APIKeyCapabilityChat,
				types.APIKeyCapabilityRetrieve,
				types.APIKeyCapabilityManageMCPServices,
			},
		},
	)
	if err != nil {
		t.Fatalf("PutExternalTenantAPIKey() error = %v", err)
	}
	if result.Created || result.Token != "persisted-runtime-token" || result.APIKey.ID != 9001 {
		t.Fatalf("PutExternalTenantAPIKey() = %#v", result)
	}
	if repository.received.ScopeType != types.APIKeyScopeTenant || repository.received.FullAccess {
		t.Fatalf("repository received invalid scope: %#v", repository.received)
	}
	if repository.received.TenantID != nil || repository.received.KnowledgeBaseIDs == nil ||
		len(repository.received.KnowledgeBaseIDs) != 0 {
		t.Fatalf("repository received caller-controlled scope: %#v", repository.received)
	}
}

func TestPutExternalTenantAPIKeyRejectsChangedReplayProtocol(t *testing.T) {
	tenantRef, _ := types.ParseExternalTenantRef("7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7")
	credentialRef, _ := types.ParseExternalTenantCredentialRef("a8af976f-47bd-5a9f-a270-4e92361e9a9d")
	repository := &externalTenantAPIKeyRepositoryStub{
		created:       false,
		storedName:    "different-runtime",
		credentialRef: credentialRef,
	}
	service := NewTenantAPIKeyService(repository)

	_, err := service.PutExternalTenantAPIKey(
		context.Background(),
		interfaces.ExternalTenantAPIKeyPutRequest{
			TenantRef: tenantRef, CredentialRef: credentialRef,
			Name:         "adax-web-runtime-v1",
			Capabilities: []types.APIKeyCapability{types.APIKeyCapabilityChat},
		},
	)
	if err != types.ErrExternalTenantCredentialConflict {
		t.Fatalf("PutExternalTenantAPIKey() error = %v", err)
	}
}

func TestPutExternalTenantAPIKeyRejectsStoredTokenHashMismatch(t *testing.T) {
	tenantRef, _ := types.ParseExternalTenantRef("7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7")
	credentialRef, _ := types.ParseExternalTenantCredentialRef("a8af976f-47bd-5a9f-a270-4e92361e9a9d")
	repository := &externalTenantAPIKeyRepositoryStub{storedKeyHash: "invalid-hash"}
	service := NewTenantAPIKeyService(repository)

	_, err := service.PutExternalTenantAPIKey(
		context.Background(),
		interfaces.ExternalTenantAPIKeyPutRequest{
			TenantRef: tenantRef, CredentialRef: credentialRef,
			Name:         "adax-web-runtime-v1",
			Capabilities: []types.APIKeyCapability{types.APIKeyCapabilityChat},
		},
	)
	if err != types.ErrExternalTenantCredentialConflict {
		t.Fatalf("PutExternalTenantAPIKey() error = %v", err)
	}
}

type externalTenantAPIKeyRepositoryStub struct {
	interfaces.TenantAPIKeyRepository
	created       bool
	storedName    string
	storedKeyHash string
	credentialRef types.ExternalTenantCredentialRef
	received      *types.TenantAPIKey
}

func (r *externalTenantAPIKeyRepositoryStub) PutExternalTenantAPIKey(
	_ context.Context,
	_ types.ExternalTenantRef,
	credentialRef types.ExternalTenantCredentialRef,
	key *types.TenantAPIKey,
) (*types.TenantAPIKey, bool, error) {
	r.received = key
	name := r.storedName
	if name == "" {
		name = key.Name
	}
	ref := credentialRef.String()
	if r.credentialRef != "" {
		ref = r.credentialRef.String()
	}
	tenantID := uint64(10001)
	keyHash := r.storedKeyHash
	if keyHash == "" {
		keyHash = hashTenantAPIKey("persisted-runtime-token")
	}
	return &types.TenantAPIKey{
		ID: 9001, TenantID: &tenantID, ExternalRef: &ref,
		ScopeType: types.APIKeyScopeTenant, Name: name,
		APIKey:       "persisted-runtime-token",
		KeyHash:      keyHash,
		Capabilities: key.Capabilities,
	}, r.created, nil
}
