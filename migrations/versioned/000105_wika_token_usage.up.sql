-- 迁移: 000105_wika_token_usage
-- 说明: 记录 Wika 用户级 PAT 的调用日聚合与最近调用事件。

DO $$ BEGIN RAISE NOTICE '[Migration 000105] Creating wika_token_usage_daily'; END $$;

CREATE TABLE IF NOT EXISTS wika_token_usage_daily (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    token_id BIGINT NOT NULL REFERENCES wika_user_tokens(id) ON DELETE CASCADE,
    tool_name VARCHAR(64) NOT NULL,
    api_method VARCHAR(12) NOT NULL,
    api_path VARCHAR(128) NOT NULL,
    day DATE NOT NULL,
    success_count BIGINT NOT NULL DEFAULT 0,
    failure_count BIGINT NOT NULL DEFAULT 0,
    last_status_code INT,
    last_error_code VARCHAR(64),
    last_success_at TIMESTAMPTZ,
    last_failure_at TIMESTAMPTZ,
    last_latency_ms INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, token_id, tool_name, day)
);

CREATE INDEX IF NOT EXISTS idx_wika_token_usage_daily_tenant_day
    ON wika_token_usage_daily(tenant_id, day DESC);

CREATE INDEX IF NOT EXISTS idx_wika_token_usage_daily_user_day
    ON wika_token_usage_daily(tenant_id, user_id, day DESC);

CREATE INDEX IF NOT EXISTS idx_wika_token_usage_daily_token_day
    ON wika_token_usage_daily(tenant_id, token_id, day DESC);

CREATE INDEX IF NOT EXISTS idx_wika_token_usage_daily_tool_day
    ON wika_token_usage_daily(tenant_id, tool_name, day DESC);

DO $$ BEGIN RAISE NOTICE '[Migration 000105] Creating wika_token_usage_events'; END $$;

CREATE TABLE IF NOT EXISTS wika_token_usage_events (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    token_id BIGINT NOT NULL REFERENCES wika_user_tokens(id) ON DELETE CASCADE,
    tool_name VARCHAR(64) NOT NULL,
    api_method VARCHAR(12) NOT NULL,
    api_path VARCHAR(128) NOT NULL,
    status_code INT NOT NULL,
    success BOOLEAN NOT NULL,
    error_code VARCHAR(64),
    latency_ms INT NOT NULL DEFAULT 0,
    knowledge_id VARCHAR(36),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wika_token_usage_events_tenant_created
    ON wika_token_usage_events(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_wika_token_usage_events_user_created
    ON wika_token_usage_events(tenant_id, user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_wika_token_usage_events_token_created
    ON wika_token_usage_events(tenant_id, token_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_wika_token_usage_events_tool_created
    ON wika_token_usage_events(tenant_id, tool_name, created_at DESC);
