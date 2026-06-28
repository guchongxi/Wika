CREATE TABLE IF NOT EXISTS wika_knowledge_versions (
  id BIGSERIAL PRIMARY KEY,
  knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id),
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id),
  version_no INT NOT NULL CHECK (version_no > 0),
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  status VARCHAR(32) NOT NULL DEFAULT '',
  review_status VARCHAR(32) NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  content_hash VARCHAR(128) NOT NULL,
  change_reason VARCHAR(64) NOT NULL DEFAULT '',
  created_by VARCHAR(64) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_wika_knowledge_versions_no
  ON wika_knowledge_versions (knowledge_id, version_no);

CREATE INDEX IF NOT EXISTS idx_wika_knowledge_versions_list
  ON wika_knowledge_versions (tenant_id, kb_id, knowledge_id, version_no DESC);
