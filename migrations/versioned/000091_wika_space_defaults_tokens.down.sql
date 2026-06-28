-- Reverse migration for 000091_wika_space_defaults_tokens.

DO $$ BEGIN RAISE NOTICE '[Migration 000091] Dropping wika_user_tokens'; END $$;

DROP INDEX IF EXISTS idx_wika_user_tokens_expires_at;
DROP INDEX IF EXISTS idx_wika_user_tokens_user_tenant_active;
DROP TABLE IF EXISTS wika_user_tokens;

DO $$ BEGIN RAISE NOTICE '[Migration 000091] Dropping wika_space_policies'; END $$;

DROP TABLE IF EXISTS wika_space_policies;

DO $$ BEGIN RAISE NOTICE '[Migration 000091] Dropping wika_space_defaults'; END $$;

DROP INDEX IF EXISTS idx_wika_space_defaults_default_kb_id;
DROP TABLE IF EXISTS wika_space_defaults;
