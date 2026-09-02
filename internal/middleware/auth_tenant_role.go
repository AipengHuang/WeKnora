package middleware

import (
	"context"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

func resolveTenantRole(
	ctx context.Context,
	memberService interfaces.TenantMemberService,
	user *types.User,
	targetTenantID uint64,
	crossTenantSwitch bool,
	cfg *config.Config,
) (types.TenantRole, bool) {
	// 1. 正常成员关系
	member, err := memberService.GetMembership(ctx, user.ID, targetTenantID)
	if err == nil && member != nil && member.Status == types.TenantMemberStatusActive {
		logger.Infof(ctx,
			"[auth] resolveTenantRole step1 hit: user=%s tenant=%d row_role=%s row_status=%s",
			user.ID, targetTenantID, member.Role, member.Status)
		return member.Role, true
	}
	if err != nil {
		logger.Warnf(ctx, "tenant_members lookup failed user=%s tenant=%d: %v",
			user.ID, targetTenantID, err)
		// Fall through; treat lookup errors the same as "no membership
		// found" so a transient DB hiccup doesn't lock everyone out.
	} else {
		var statusInfo string
		if member == nil {
			statusInfo = "no_row"
		} else {
			statusInfo = "row_exists status=" + string(member.Status) + " role=" + string(member.Role)
		}
		logger.Warnf(ctx,
			"[auth] resolveTenantRole step1 miss: user=%s tenant=%d (%s)",
			user.ID, targetTenantID, statusInfo)
	}

	// 2. 跨空间超管直通：CanAccessAllTenants 用户切到别的空间时不强制要求 membership。
	//    注意：这里只授予临时 Admin 角色，不写入 tenant_members，避免"看一眼别人空间"
	//    意外升级为持久化所有权。
	if crossTenantSwitch && user.CanAccessAllTenants {
		logger.Infof(ctx,
			"[auth] resolveTenantRole step2 (cross-tenant superuser) -> Admin: user=%s tenant=%d",
			user.ID, targetTenantID)
		return types.TenantRoleAdmin, true
	}

	// 3. 孤儿空间自愈：仅当用户登录的是自己的 home tenant、且该空间尚无任何活跃成员时
	//    允许自动晋升为 Owner。跨空间 switch / JWT 指向他人空间的场景一律不进入此分支，
	//    防止越权获得他人空间的 Owner 权限。
	isHomeTenant := !crossTenantSwitch && targetTenantID == user.TenantID
	if isHomeTenant {
		hasAny, anyErr := memberService.HasAnyMembers(ctx, targetTenantID)
		if anyErr == nil && !hasAny {
			if _, e := memberService.AddMember(
				ctx, user.ID, targetTenantID, types.TenantRoleOwner, nil,
			); e == nil {
				logger.Infof(ctx,
					"[audit] Auto-promoted user %s to Owner of orphan tenant %d (home_tenant=true)",
					user.ID, targetTenantID,
				)
				return types.TenantRoleOwner, true
			} else {
				logger.Warnf(ctx, "Failed to auto-promote user %s in tenant %d: %v",
					user.ID, targetTenantID, e)
			}
		}
	}

	// 4. 兜底：根据 EnableRBAC 决定 fail-closed 还是 fail-open
	if cfg != nil && cfg.Tenant.IsRBACEnforced() {
		logger.Warnf(ctx,
			"[auth] resolveTenantRole step4 fail-closed (EnableRBAC=true): user=%s tenant=%d",
			user.ID, targetTenantID)
		return "", false
	}
	logger.Warnf(ctx,
		"[auth] resolveTenantRole step4 fail-open (EnableRBAC=false) -> Admin: user=%s tenant=%d",
		user.ID, targetTenantID)
	// fail-open 期间保持现有行为（每个登录用户在自己空间里都是"管理员"）。
	return types.TenantRoleAdmin, true
}
