-- Reverse migration for 000090_wika_space.

DO $$ BEGIN RAISE NOTICE '[Migration 000090] Dropping user_personal_spaces'; END $$;

DROP INDEX IF EXISTS idx_user_personal_spaces_tenant_id;
DROP TABLE IF EXISTS user_personal_spaces;

DO $$ BEGIN RAISE NOTICE '[Migration 000090] Removing tenants.space_type'; END $$;

DROP INDEX IF EXISTS idx_tenants_space_type;

ALTER TABLE tenants
    DROP CONSTRAINT IF EXISTS chk_tenants_space_type;

ALTER TABLE tenants
    DROP COLUMN IF EXISTS space_type;
