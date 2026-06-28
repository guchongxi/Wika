-- Migration: 000090_wika_space
-- Description: Add Wika space type and one-person personal space mapping.

DO $$ BEGIN RAISE NOTICE '[Migration 000090] Adding tenants.space_type'; END $$;

ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS space_type VARCHAR(16) NOT NULL DEFAULT 'team';

UPDATE tenants
SET space_type = 'team'
WHERE space_type IS NULL OR space_type = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_tenants_space_type'
    ) THEN
        ALTER TABLE tenants
            ADD CONSTRAINT chk_tenants_space_type
            CHECK (space_type IN ('personal', 'team'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_tenants_space_type
    ON tenants(space_type);

DO $$ BEGIN RAISE NOTICE '[Migration 000090] Creating user_personal_spaces'; END $$;

CREATE TABLE IF NOT EXISTS user_personal_spaces (
    user_id VARCHAR(36) PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    tenant_id BIGINT NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_personal_spaces_tenant_id
    ON user_personal_spaces(tenant_id);
