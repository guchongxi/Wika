ALTER TABLE wika_org_shares
  DROP CONSTRAINT IF EXISTS chk_wika_org_shares_allowed_fields;

ALTER TABLE wika_org_shares
  ADD CONSTRAINT chk_wika_org_shares_allowed_fields
  CHECK (
    jsonb_typeof(allowed_fields) = 'array'
    AND allowed_fields <@ '["id","title","source_tenant_id","source_kb_id","quality_score","freshness_status"]'::jsonb
  );
