package auth

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func setupTokenUsageStoreTestDB(t *testing.T) *GormTokenStore {
	t.Helper()
	db := setupTokenStoreTestDB(t)
	require.NoError(t, db.AutoMigrate(&types.WikaTokenUsageDaily{}, &types.WikaTokenUsageEvent{}))
	require.NoError(t, db.Create(&types.User{
		ID:           "user-1",
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "x",
		TenantID:     7,
		IsActive:     true,
	}).Error)
	require.NoError(t, db.Create(&types.User{
		ID:           "user-2",
		Username:     "bob",
		Email:        "bob@example.com",
		PasswordHash: "x",
		TenantID:     7,
		IsActive:     true,
	}).Error)
	require.NoError(t, db.Create(&types.Tenant{ID: 7, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	return NewGormTokenStore(db)
}

func saveUsageToken(t *testing.T, store *GormTokenStore, userID string, tokenID uint64, name string) {
	t.Helper()
	require.NoError(t, store.SaveToken(context.Background(), &types.WikaUserToken{
		ID:          tokenID,
		UserID:      userID,
		TenantID:    7,
		Name:        name,
		TokenPrefix: "wika_pat_" + name,
		TokenHash:   "hash-" + name,
		HashAlg:     "sha256_pepper",
		Scopes:      types.JSON([]byte(`["knowledge:push","knowledge:read"]`)),
		ExpiresAt:   time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		CreatedAt:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}))
}

func TestGormTokenStoreRecordUsageAggregatesAndWritesEvents(t *testing.T) {
	store := setupTokenUsageStoreTestDB(t)
	saveUsageToken(t, store, "user-1", 11, "claude")
	ctx := context.Background()
	first := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	second := first.Add(2 * time.Minute)

	require.NoError(t, store.RecordUsage(ctx, TokenUsageRecord{
		TenantID:    7,
		UserID:      "user-1",
		TokenID:     11,
		ToolName:    "push_knowledge",
		APIMethod:   "POST",
		APIPath:     "/api/v1/wika/knowledge/push",
		StatusCode:  200,
		Success:     true,
		LatencyMS:   123,
		KnowledgeID: "knowledge-1",
		OccurredAt:  first,
	}))
	require.NoError(t, store.RecordUsage(ctx, TokenUsageRecord{
		TenantID:   7,
		UserID:     "user-1",
		TokenID:    11,
		ToolName:   "push_knowledge",
		APIMethod:  "POST",
		APIPath:    "/api/v1/wika/knowledge/push",
		StatusCode: 500,
		Success:    false,
		ErrorCode:  "internal_error",
		LatencyMS:  456,
		OccurredAt: second,
	}))

	result, err := store.ListUsage(ctx, TokenUsageFilter{
		TenantID:     7,
		ViewerUserID: "user-1",
		Scope:        UsageScopeMine,
		From:         time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
		To:           time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
		Limit:        20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Len(t, result.Items, 1)
	item := result.Items[0]
	require.Equal(t, uint64(11), item.Token.ID)
	require.Equal(t, "alice", item.Token.OwnerUsername)
	require.Equal(t, int64(2), item.Summary.TotalCalls)
	require.Equal(t, int64(1), item.Summary.SuccessCount)
	require.Equal(t, int64(1), item.Summary.FailureCount)
	require.Equal(t, 500, item.Summary.LastStatusCode)
	require.Equal(t, "internal_error", item.Summary.LastErrorCode)
	require.Equal(t, "error", item.Token.ConnectionStatus)
	require.Len(t, item.Summary.Tools, 1)
	require.Equal(t, "push_knowledge", item.Summary.Tools[0].ToolName)

	events, err := store.ListUsageEvents(ctx, TokenUsageFilter{
		TenantID:     7,
		ViewerUserID: "user-1",
		Scope:        UsageScopeMine,
		Limit:        10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), events.Total)
	require.Len(t, events.Events, 2)
	require.Equal(t, "internal_error", events.Events[0].ErrorCode)
	require.Equal(t, "knowledge-1", events.Events[1].KnowledgeID)

	tokens, err := store.ListTokens(ctx, "user-1", 7)
	require.NoError(t, err)
	require.NotNil(t, tokens[0].LastUsedAt)
	require.True(t, tokens[0].LastUsedAt.Equal(second))
}

func TestGormTokenStoreListUsageScopesMineAndAll(t *testing.T) {
	store := setupTokenUsageStoreTestDB(t)
	saveUsageToken(t, store, "user-1", 11, "claude")
	saveUsageToken(t, store, "user-2", 12, "cursor")
	ctx := context.Background()
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	for _, rec := range []TokenUsageRecord{
		{TenantID: 7, UserID: "user-1", TokenID: 11, ToolName: "push_knowledge", APIMethod: "POST", APIPath: "/api/v1/wika/knowledge/push", StatusCode: 200, Success: true, OccurredAt: now},
		{TenantID: 7, UserID: "user-2", TokenID: 12, ToolName: "get_my_knowledge", APIMethod: "GET", APIPath: "/api/v1/wika/knowledge/mine", StatusCode: 200, Success: true, OccurredAt: now},
	} {
		require.NoError(t, store.RecordUsage(ctx, rec))
	}

	mine, err := store.ListUsage(ctx, TokenUsageFilter{
		TenantID:     7,
		ViewerUserID: "user-1",
		Scope:        UsageScopeMine,
		Limit:        20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), mine.Total)
	require.Equal(t, "user-1", mine.Items[0].Token.OwnerUserID)

	all, err := store.ListUsage(ctx, TokenUsageFilter{
		TenantID: 7,
		Scope:    UsageScopeAll,
		Limit:    20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), all.Total)

	filtered, err := store.ListUsage(ctx, TokenUsageFilter{
		TenantID:    7,
		Scope:       UsageScopeAll,
		OwnerUserID: "user-2",
		ToolName:    "get_my_knowledge",
		Limit:       20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), filtered.Total)
	require.Equal(t, uint64(12), filtered.Items[0].Token.ID)
	require.Empty(t, filtered.Items[0].Token.TokenHash)
}
