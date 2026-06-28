package auth

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTokenStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.User{}, &types.Tenant{}, &types.WikaUserToken{}))
	return db
}

func TestGormTokenStoreSavesAndFindsByHash(t *testing.T) {
	db := setupTokenStoreTestDB(t)
	require.NoError(t, db.Create(&types.User{
		ID:           "user-1",
		Username:     "user1",
		Email:        "user1@example.com",
		PasswordHash: "x",
	}).Error)
	require.NoError(t, db.Create(&types.Tenant{ID: 7, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	store := NewGormTokenStore(db)

	err := store.SaveToken(context.Background(), &types.WikaUserToken{
		UserID:      "user-1",
		TenantID:    7,
		Name:        "Claude Code",
		TokenPrefix: "wika_pat_abc",
		TokenHash:   "hash-1",
		HashAlg:     "sha256_pepper",
		Scopes:      types.JSON([]byte(`["knowledge:search"]`)),
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	token, err := store.FindTokenByHash(context.Background(), "hash-1")

	require.NoError(t, err)
	if token.UserID != "user-1" || token.TenantID != 7 || token.TokenHash != "hash-1" {
		t.Fatalf("unexpected token: %+v", token)
	}
}

func TestGormTokenStoreFindMissingHashReturnsInvalid(t *testing.T) {
	db := setupTokenStoreTestDB(t)
	store := NewGormTokenStore(db)

	_, err := store.FindTokenByHash(context.Background(), "missing")

	if err != ErrTokenInvalid {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}
}
