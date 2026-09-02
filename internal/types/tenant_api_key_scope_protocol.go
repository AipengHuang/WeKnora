package types

import "fmt"

// ParseAPIKeyScopeType 只接受协议声明的精确作用域值。
func ParseAPIKeyScopeType(value APIKeyScopeType) (APIKeyScopeType, error) {
	switch value {
	case APIKeyScopeTenant:
		return APIKeyScopeTenant, nil
	case APIKeyScopePlatform:
		return APIKeyScopePlatform, nil
	default:
		return "", fmt.Errorf("unknown API key scope type %q", value)
	}
}
