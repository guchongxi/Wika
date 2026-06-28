CREATE TABLE IF NOT EXISTS wika_graph_entities (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id),
  entity_key VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  entity_type VARCHAR(64) NOT NULL,
  summary TEXT,
  source_knowledge_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  confidence_score NUMERIC(5,4) NOT NULL DEFAULT 0 CHECK (confidence_score >= 0 AND confidence_score <= 1),
  last_extracted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS wika_graph_edges (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id),
  source_entity_id BIGINT NOT NULL REFERENCES wika_graph_entities(id),
  target_entity_id BIGINT NOT NULL REFERENCES wika_graph_entities(id),
  relation_type VARCHAR(64) NOT NULL,
  evidence_knowledge_id VARCHAR(36) REFERENCES knowledges(id),
  evidence_chunk_id VARCHAR(36),
  evidence_text TEXT,
  confidence_score NUMERIC(5,4) NOT NULL DEFAULT 0 CHECK (confidence_score >= 0 AND confidence_score <= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_wika_graph_entities_key
  ON wika_graph_entities (tenant_id, kb_id, entity_key);

CREATE INDEX IF NOT EXISTS idx_wika_graph_entities_type_name
  ON wika_graph_entities (tenant_id, kb_id, entity_type, name);

CREATE INDEX IF NOT EXISTS idx_wika_graph_edges_source
  ON wika_graph_edges (tenant_id, kb_id, source_entity_id);

CREATE INDEX IF NOT EXISTS idx_wika_graph_edges_target
  ON wika_graph_edges (tenant_id, kb_id, target_entity_id);

CREATE INDEX IF NOT EXISTS idx_wika_graph_edges_evidence_knowledge
  ON wika_graph_edges (evidence_knowledge_id);
