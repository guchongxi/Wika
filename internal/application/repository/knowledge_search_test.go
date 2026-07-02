package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const knowledgeSearchTestDDL = `
CREATE TABLE IF NOT EXISTS knowledge_bases (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	type TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS knowledges (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	knowledge_base_id TEXT NOT NULL,
	type TEXT DEFAULT '',
	title TEXT DEFAULT '',
	description TEXT DEFAULT '',
	source TEXT DEFAULT '',
	parse_status TEXT DEFAULT '',
	enable_status TEXT DEFAULT '',
	embedding_model_id TEXT DEFAULT '',
	file_name TEXT DEFAULT '',
	file_type TEXT DEFAULT '',
	file_size INTEGER DEFAULT 0,
	file_hash TEXT DEFAULT '',
	file_path TEXT DEFAULT '',
	storage_size INTEGER DEFAULT 0,
	metadata JSON DEFAULT '{}',
	last_faq_import_result JSON DEFAULT '{}',
	created_at DATETIME,
	updated_at DATETIME,
	processed_at DATETIME,
	error_message TEXT DEFAULT '',
	deleted_at DATETIME
);
`

func newKnowledgeSearchTestRepo(t *testing.T) *knowledgeRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(knowledgeSearchTestDDL).Error)
	return NewKnowledgeRepository(db).(*knowledgeRepository)
}

func TestSearchKnowledgeInScopesMatchesManualMetadataContent(t *testing.T) {
	repo := newKnowledgeSearchTestRepo(t)
	now := time.Date(2026, 6, 29, 15, 0, 0, 0, time.UTC)
	require.NoError(t, repo.db.Exec(
		`INSERT INTO knowledge_bases (id, tenant_id, name, type) VALUES (?, ?, ?, ?)`,
		"kb-personal", uint64(70), "个人空间", types.KnowledgeBaseTypeDocument,
	).Error)
	manualMeta, err := types.NewManualKnowledgeMetadata(
		"排查服务启动失败时先检查管理员全局默认模型和 KB defaults。",
		types.ManualKnowledgeStatusPublish,
		1,
	).ToJSON()
	require.NoError(t, err)
	require.NoError(t, repo.db.Exec(
		`INSERT INTO knowledges (
			id, tenant_id, knowledge_base_id, type, title, source, parse_status,
			enable_status, file_name, file_type, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"k-manual", uint64(70), "kb-personal", types.KnowledgeTypeManual, "个人知识",
		types.KnowledgeTypeManual, "pending", "disabled", "个人知识.md", types.KnowledgeTypeManual,
		manualMeta, now, now,
	).Error)

	got, hasMore, err := repo.SearchKnowledgeInScopes(context.Background(), []types.KnowledgeSearchScope{
		{TenantID: 70, KBID: "kb-personal"},
	}, "管理员全局默认模型", 0, 5, nil)

	require.NoError(t, err)
	require.False(t, hasMore)
	require.Len(t, got, 1)
	require.Equal(t, "k-manual", got[0].ID)
}
