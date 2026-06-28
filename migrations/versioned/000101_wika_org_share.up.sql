CREATE TABLE IF NOT EXISTS wika_org_shares (
  id BIGSERIAL PRIMARY KEY,
  org_id VARCHAR(36) NOT NULL REFERENCES organizations(id),
  source_tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  source_kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id),
  target_tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  mode VARCHAR(24) NOT NULL DEFAULT 'reference'
    CHECK (mode IN ('reference', 'snapshot')),
  allowed_fields JSONB NOT NULL DEFAULT '[]'::jsonb,
  status VARCHAR(24) NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'active', 'revoked')),
  created_by VARCHAR(64) NOT NULL,
  accepted_by VARCHAR(64),
  accepted_at TIMESTAMPTZ,
  revoked_by VARCHAR(64),
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_wika_org_shares_open
  ON wika_org_shares (source_kb_id, target_tenant_id)
  WHERE status IN ('pending', 'active');

CREATE INDEX IF NOT EXISTS idx_wika_org_shares_target_status
  ON wika_org_shares (target_tenant_id, status);

CREATE INDEX IF NOT EXISTS idx_wika_org_shares_source_status
  ON wika_org_shares (source_tenant_id, source_kb_id, status);
