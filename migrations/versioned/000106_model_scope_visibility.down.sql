-- Down migration for 000106_model_scope_visibility.

DROP INDEX IF EXISTS idx_models_system_default_type;
DROP INDEX IF EXISTS idx_models_system_selectable_type;
DROP INDEX IF EXISTS idx_models_owner_user_type;
DROP INDEX IF EXISTS idx_models_scope_type;

ALTER TABLE models DROP CONSTRAINT IF EXISTS chk_models_scope;

ALTER TABLE models
    DROP COLUMN IF EXISTS user_selectable,
    DROP COLUMN IF EXISTS owner_user_id,
    DROP COLUMN IF EXISTS scope;
