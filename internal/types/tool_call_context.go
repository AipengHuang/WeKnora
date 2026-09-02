package types

import "context"

// ToolCallIdentity 是工具执行与产物持久化共用的稳定身份。
type ToolCallIdentity struct {
	AssistantMessageID string
	ToolCallID         string
}

func (identity ToolCallIdentity) IsValid() bool {
	return identity.AssistantMessageID != "" && identity.ToolCallID != ""
}

type toolCallIdentityContextKey struct{}

// WithToolCallIdentity 将当前工具调用身份写入执行上下文。
func WithToolCallIdentity(ctx context.Context, identity ToolCallIdentity) context.Context {
	if ctx == nil || !identity.IsValid() {
		return ctx
	}
	return context.WithValue(ctx, toolCallIdentityContextKey{}, identity)
}

// ToolCallIdentityFromContext 读取当前工具调用身份。
func ToolCallIdentityFromContext(ctx context.Context) (ToolCallIdentity, bool) {
	if ctx == nil {
		return ToolCallIdentity{}, false
	}
	identity, ok := ctx.Value(toolCallIdentityContextKey{}).(ToolCallIdentity)
	return identity, ok && identity.IsValid()
}
