CREATE TABLE IF NOT EXISTS wika_conflict_checks (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id),
  trigger VARCHAR(32) NOT NULL,
  status VARCHAR(24) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
  attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  locked_until TIMESTAMPTZ,
  locked_by VARCHAR(128),
  created_by VARCHAR(64) NOT NULL,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_msg TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS wika_conflict_items (
  id BIGSERIAL PRIMARY KEY,
  check_id BIGINT NOT NULL REFERENCES wika_conflict_checks(id),
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id),
  source_knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id),
  target_knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id),
  conflict_type VARCHAR(32) NOT NULL CHECK (conflict_type IN ('contradiction', 'duplicate', 'outdated', 'scope_overlap')),
  confidence_score NUMERIC(5,4) NOT NULL DEFAULT 0 CHECK (confidence_score >= 0 AND confidence_score <= 1),
  evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
  ai_explanation TEXT,
  status VARCHAR(24) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'confirmed', 'dismissed', 'resolved')),
  reviewer_comment TEXT,
  resolved_by VARCHAR(64),
  resolved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wika_conflict_checks_kb_created
  ON wika_conflict_checks (tenant_id, kb_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_wika_conflict_checks_worker
  ON wika_conflict_checks (status, locked_until, created_at)
  WHERE status IN ('pending', 'failed');

CREATE UNIQUE INDEX IF NOT EXISTS ux_wika_conflict_items_open
  ON wika_conflict_items (tenant_id, kb_id, source_knowledge_id, target_knowledge_id, conflict_type)
  WHERE status IN ('open', 'confirmed');

CREATE INDEX IF NOT EXISTS idx_wika_conflict_items_status
  ON wika_conflict_items (tenant_id, kb_id, status, updated_at DESC);
