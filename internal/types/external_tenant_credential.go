package types

import "errors"

// ExternalTenantCredentialRef 是平台控制面提交的规范 UUID 凭据标识。
type ExternalTenantCredentialRef string

var (
	ErrExternalTenantNotFound           = errors.New("external tenant is not found")
	ErrExternalTenantCredentialProtocol = errors.New("external tenant credential protocol is invalid")
	ErrExternalTenantCredentialConflict = errors.New("external tenant credential conflicts with stored protocol")
)

// ParseExternalTenantCredentialRef 只接受 UUID 库生成的规范小写文本形式。
func ParseExternalTenantCredentialRef(value string) (ExternalTenantCredentialRef, error) {
	parsed, err := parseCanonicalExternalUUID(value, "external tenant credential reference")
	return ExternalTenantCredentialRef(parsed), err
}

func (r ExternalTenantCredentialRef) String() string { return string(r) }
