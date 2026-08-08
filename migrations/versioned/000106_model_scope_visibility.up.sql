-- Migration: 000106_model_scope_visibility
-- 为模型补充系统/租户/用户作用域、用户归属和用户可选性。

DO $$ BEGIN RAISE NOTICE '[Migration 000106] Adding model scope and visibility columns'; END $$;

ALTER TABLE models
    ADD COLUMN IF NOT EXISTS scope VARCHAR(16) NOT NULL DEFAULT 'tenant',
    ADD COLUMN IF NOT EXISTS owner_user_id VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS user_selectable BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE models
SET scope = 'system',
    user_selectable = TRUE
WHERE is_builtin = TRUE;

UPDATE models
SET scope = 'tenant'
WHERE is_builtin = FALSE
  AND scope = 'tenant';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_models_scope'
    ) THEN
        ALTER TABLE models
            ADD CONSTRAINT chk_models_scope CHECK (scope IN ('system', 'tenant', 'user'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_models_scope_type
    ON models(scope, type);

CREATE INDEX IF NOT EXISTS idx_models_owner_user_type
    ON models(owner_user_id, type)
    WHERE scope = 'user' AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_models_system_selectable_type
    ON models(type, user_selectable)
    WHERE scope = 'system' AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_models_system_default_type
    ON models(type)
    WHERE scope = 'system' AND is_default = TRUE AND deleted_at IS NULL;
