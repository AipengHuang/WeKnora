package types

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ExternalTenantRef 是平台控制面提交的规范 UUID 租户标识。
type ExternalTenantRef string

var ErrExternalTenantDeleted = errors.New("external tenant is deleted")

// ParseExternalTenantRef 只接受 UUID 库生成的规范小写文本形式。
func ParseExternalTenantRef(value string) (ExternalTenantRef, error) {
	parsed, err := parseCanonicalExternalUUID(value, "external tenant reference")
	return ExternalTenantRef(parsed), err
}

func (r ExternalTenantRef) String() string { return string(r) }

func parseCanonicalExternalUUID(value, name string) (string, error) {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.String() != value {
		return "", fmt.Errorf("%s must be a canonical UUID", name)
	}
	return value, nil
}
