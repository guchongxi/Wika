package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	wikaspace "github.com/Tencent/WeKnora/internal/wika/space"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserRegisterPersonalSpaceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.User{},
		&types.Tenant{},
		&types.TenantMember{},
		&types.UserPersonalSpace{},
		&types.KnowledgeBase{},
		&types.WikaSpaceDefault{},
		&types.WikaSpacePolicy{},
	))
	return db
}

func TestRegisterCreatesPersonalSpaceAndDefaultKnowledgeBase(t *testing.T) {
	t.Setenv("TENANT_AES_KEY", "12345678901234567890123456789012")
	db := setupUserRegisterPersonalSpaceDB(t)
	tenantSvc := NewTenantService(repository.NewTenantRepository(db))
	memberSvc := NewTenantMemberService(repository.NewTenantMemberRepository(db), nil)
	personalSpaces := wikaspace.NewService(wikaspace.NewGormStore(db))
	svc := &userService{
		userRepo:       repository.NewUserRepository(db),
		tokenRepo:      repository.NewAuthTokenRepository(db),
		tenantService:  tenantSvc,
		memberService:  memberSvc,
		personalSpaces: personalSpaces,
	}

	user, err := svc.Register(context.Background(), &types.RegisterRequest{
		Username: "wika-register-personal",
		Email:    "wika-register-personal@example.com",
		Password: "Wika123456",
	})
	require.NoError(t, err)

	var mapping types.UserPersonalSpace
	require.NoError(t, db.First(&mapping, "user_id = ?", user.ID).Error)

	var personalTenant types.Tenant
	require.NoError(t, db.First(&personalTenant, "id = ?", mapping.TenantID).Error)
	require.Equal(t, types.SpaceTypePersonal, personalTenant.SpaceType)

	var personalMember types.TenantMember
	require.NoError(t, db.First(&personalMember, "user_id = ? AND tenant_id = ?", user.ID, personalTenant.ID).Error)
	require.Equal(t, types.TenantRoleOwner, personalMember.Role)

	var defaults types.WikaSpaceDefault
	require.NoError(t, db.First(&defaults, "tenant_id = ?", personalTenant.ID).Error)

	var kb types.KnowledgeBase
	require.NoError(t, db.First(&kb, "id = ?", defaults.DefaultKBID).Error)
	require.Equal(t, user.ID, kb.CreatorID)
	require.Equal(t, types.KnowledgeBaseTypeDocument, kb.Type)

	var teamDefaults types.WikaSpaceDefault
	require.NoError(t, db.First(&teamDefaults, "tenant_id = ?", user.TenantID).Error)

	var teamKB types.KnowledgeBase
	require.NoError(t, db.First(&teamKB, "id = ?", teamDefaults.DefaultKBID).Error)
	require.Equal(t, user.TenantID, teamKB.TenantID)
	require.Equal(t, user.ID, teamKB.CreatorID)
	require.Equal(t, types.KnowledgeBaseTypeDocument, teamKB.Type)

	var teamPolicy types.WikaSpacePolicy
	require.NoError(t, db.First(&teamPolicy, "tenant_id = ?", user.TenantID).Error)
	require.False(t, teamPolicy.AutoApplyApproved)
	require.Equal(t, uint64(1), teamPolicy.PolicyVersion)
}
