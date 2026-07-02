package graph

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGraphStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.Tenant{},
		&types.KnowledgeBase{},
		&types.Knowledge{},
		&types.WikaGraphEntity{},
		&types.WikaGraphEdge{},
	))
	return db
}

func TestGormGraphStoreListsEntitiesEdgesAndOverview(t *testing.T) {
	db := setupGraphStoreTestDB(t)
	require.NoError(t, db.Create(&types.Tenant{ID: 80, Name: "team", SpaceType: types.SpaceTypeTeam}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-team", TenantID: 80, Type: types.KnowledgeBaseTypeDocument}).Error)
	require.NoError(t, db.Create(&types.Knowledge{ID: "k-1", TenantID: 80, KnowledgeBaseID: "kb-team", Title: "索引排查"}).Error)
	require.NoError(t, db.Create(&types.WikaGraphEntity{
		TenantID: 80, KBID: "kb-team", EntityKey: "concept:index-delay", Name: "索引延迟", EntityType: "concept",
		SourceKnowledgeIDs: types.JSON([]byte(`["k-1"]`)), ConfidenceScore: 0.91,
	}).Error)
	require.NoError(t, db.Create(&types.WikaGraphEntity{
		TenantID: 80, KBID: "kb-team", EntityKey: "system:embedding", Name: "Embedding", EntityType: "system",
		SourceKnowledgeIDs: types.JSON([]byte(`["k-1"]`)), ConfidenceScore: 0.86,
	}).Error)

	var source, target types.WikaGraphEntity
	require.NoError(t, db.First(&source, "entity_key = ?", "concept:index-delay").Error)
	require.NoError(t, db.First(&target, "entity_key = ?", "system:embedding").Error)
	require.NoError(t, db.Create(&types.WikaGraphEdge{
		TenantID: 80, KBID: "kb-team", SourceEntityID: source.ID, TargetEntityID: target.ID, RelationType: "depends_on",
		EvidenceKnowledgeID: "k-1", EvidenceChunkID: "c-1", EvidenceText: "索引依赖 embedding 完成", ConfidenceScore: 0.88,
	}).Error)

	store := NewGormStore(db)
	overview, err := store.Overview(context.Background(), 80, "kb-team")
	require.NoError(t, err)
	if overview.EntityCount != 2 || overview.EdgeCount != 1 {
		t.Fatalf("unexpected overview: %+v", overview)
	}
	systemOverview, err := store.OverviewByKB(context.Background(), "kb-team")
	require.NoError(t, err)
	if systemOverview.TenantID != 80 || systemOverview.EntityCount != 2 || systemOverview.EdgeCount != 1 {
		t.Fatalf("unexpected system overview: %+v", systemOverview)
	}

	entities, total, err := store.ListEntities(context.Background(), ListEntitiesInput{
		TenantID: 80,
		KBID:     "kb-team",
		Limit:    10,
	})
	require.NoError(t, err)
	if total != 2 || len(entities) != 2 {
		t.Fatalf("unexpected entities total=%d items=%+v", total, entities)
	}

	entity, err := store.GetEntity(context.Background(), 80, "kb-team", source.ID)
	require.NoError(t, err)
	if entity.Name != "索引延迟" {
		t.Fatalf("unexpected entity: %+v", entity)
	}

	edges, edgeTotal, err := store.ListEdges(context.Background(), ListEdgesInput{
		TenantID: 80,
		KBID:     "kb-team",
		Limit:    10,
	})
	require.NoError(t, err)
	if edgeTotal != 1 || len(edges) != 1 || edges[0].EvidenceText != "索引依赖 embedding 完成" {
		t.Fatalf("unexpected edges total=%d items=%+v", edgeTotal, edges)
	}
}
