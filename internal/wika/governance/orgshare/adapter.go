package orgshare

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type tenantMemberAdminChecker struct {
	members interfaces.TenantMemberService
}

func NewTenantMemberAdminChecker(members interfaces.TenantMemberService) TeamAdminChecker {
	return &tenantMemberAdminChecker{members: members}
}

func (c *tenantMemberAdminChecker) CanAdminTenant(ctx context.Context, actorID string, tenantID uint64) bool {
	if c == nil || c.members == nil {
		return false
	}
	member, err := c.members.GetMembership(ctx, actorID, tenantID)
	if err != nil || member == nil {
		return false
	}
	return member.Role.HasPermission(types.TenantRoleAdmin)
}
