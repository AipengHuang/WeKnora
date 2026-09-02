package types

import "fmt"

// ParseAPIKeyCapability 只接受协议声明的精确能力值，不修剪或转换输入。
func ParseAPIKeyCapability(value string) (APIKeyCapability, error) {
	switch APIKeyCapability(value) {
	case APIKeyCapabilityRetrieve:
		return APIKeyCapabilityRetrieve, nil
	case APIKeyCapabilityChat:
		return APIKeyCapabilityChat, nil
	case APIKeyCapabilityReadAgents:
		return APIKeyCapabilityReadAgents, nil
	case APIKeyCapabilityIngest:
		return APIKeyCapabilityIngest, nil
	case APIKeyCapabilityManageKnowledgeBases:
		return APIKeyCapabilityManageKnowledgeBases, nil
	case APIKeyCapabilityManageAgents:
		return APIKeyCapabilityManageAgents, nil
	case APIKeyCapabilityMessageHistory:
		return APIKeyCapabilityMessageHistory, nil
	case APIKeyCapabilityManageModels:
		return APIKeyCapabilityManageModels, nil
	case APIKeyCapabilityManageMCPServices:
		return APIKeyCapabilityManageMCPServices, nil
	case APIKeyCapabilityManageSandboxes:
		return APIKeyCapabilityManageSandboxes, nil
	case APIKeyCapabilityManageDataSources:
		return APIKeyCapabilityManageDataSources, nil
	case APIKeyCapabilityManageChannels:
		return APIKeyCapabilityManageChannels, nil
	case APIKeyCapabilityManageVectorStores:
		return APIKeyCapabilityManageVectorStores, nil
	case APIKeyCapabilityManageStorageBackends:
		return APIKeyCapabilityManageStorageBackends, nil
	case APIKeyCapabilityManageWebSearch:
		return APIKeyCapabilityManageWebSearch, nil
	case APIKeyCapabilityRunEvaluations:
		return APIKeyCapabilityRunEvaluations, nil
	case APIKeyCapabilityManageMembers:
		return APIKeyCapabilityManageMembers, nil
	case APIKeyCapabilityManageSpaces:
		return APIKeyCapabilityManageSpaces, nil
	case APIKeyCapabilityManageTenantSettings:
		return APIKeyCapabilityManageTenantSettings, nil
	case APIKeyCapabilitySystemTenantsRead:
		return APIKeyCapabilitySystemTenantsRead, nil
	case APIKeyCapabilitySystemTenantsManage:
		return APIKeyCapabilitySystemTenantsManage, nil
	case APIKeyCapabilitySystemSettingsRead:
		return APIKeyCapabilitySystemSettingsRead, nil
	case APIKeyCapabilitySystemSettingsManage:
		return APIKeyCapabilitySystemSettingsManage, nil
	case APIKeyCapabilitySystemRuntimeRead:
		return APIKeyCapabilitySystemRuntimeRead, nil
	case APIKeyCapabilitySystemRuntimeManage:
		return APIKeyCapabilitySystemRuntimeManage, nil
	case APIKeyCapabilitySystemAuditRead:
		return APIKeyCapabilitySystemAuditRead, nil
	default:
		return "", fmt.Errorf("unknown API key capability %q", value)
	}
}

// ParseAPIKeyCapabilities 校验整个能力集合，并明确拒绝重复项。
func ParseAPIKeyCapabilities(values StringArray) (StringArray, error) {
	parsed := make(StringArray, 0, len(values))
	seen := make(map[APIKeyCapability]struct{}, len(values))
	for _, value := range values {
		capability, err := ParseAPIKeyCapability(value)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[capability]; exists {
			return nil, fmt.Errorf("duplicate API key capability %q", value)
		}
		seen[capability] = struct{}{}
		parsed = append(parsed, string(capability))
	}
	return parsed, nil
}

// NormalizeAPIKeyCapability 为现有调用点提供精确投影；非法值返回空值并由调用点拒绝。
func NormalizeAPIKeyCapability(capability APIKeyCapability) APIKeyCapability {
	parsed, err := ParseAPIKeyCapability(string(capability))
	if err != nil {
		return ""
	}
	return parsed
}

// NormalizeAPIKeyCapabilities 不改写输入；任一值非法或重复时返回空集合并由调用点拒绝。
func NormalizeAPIKeyCapabilities(capabilities StringArray) StringArray {
	parsed, err := ParseAPIKeyCapabilities(capabilities)
	if err != nil {
		return nil
	}
	return parsed
}
