-- Migration: 000091_wika_space_defaults_tokens
-- Description: Add Wika space defaults, suggestion policy, and user-level MCP tokens.

DO $$ BEGIN RAISE NOTICE '[Migration 000091] Creating wika_space_defaults'; END $$;

CREATE TABLE IF NOT EXISTS wika_space_defaults (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    default_kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE RESTRICT,
    created_by VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wika_space_defaults_default_kb_id
    ON wika_space_defaults(default_kb_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000091] Creating wika_space_policies'; END $$;

CREATE TABLE IF NOT EXISTS wika_space_policies (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    auto_apply_approved BOOLEAN NOT NULL DEFAULT FALSE,
    policy_version BIGINT NOT NULL DEFAULT 1,
    safety_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$ BEGIN RAISE NOTICE '[Migration 000091] Creating wika_user_tokens'; END $$;

CREATE TABLE IF NOT EXISTS wika_user_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    token_prefix VARCHAR(32) NOT NULL,
    token_hash VARCHAR(128) NOT NULL UNIQUE,
    hash_alg VARCHAR(32) NOT NULL DEFAULT 'sha256_pepper',
    scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_ip INET,
    last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_wika_user_tokens_user_tenant_active
    ON wika_user_tokens(user_id, tenant_id)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_wika_user_tokens_expires_at
    ON wika_user_tokens(expires_at);
