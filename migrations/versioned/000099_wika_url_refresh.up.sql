CREATE TABLE IF NOT EXISTS wika_url_refresh_jobs (
  id BIGSERIAL PRIMARY KEY,
  schedule_id BIGINT,
  knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id),
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id),
  source_url TEXT NOT NULL,
  scheduled_for TIMESTAMPTZ,
  idempotency_key VARCHAR(128),
  status VARCHAR(24) NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'running', 'pending_review', 'applied', 'rejected', 'failed')),
  fetched_hash VARCHAR(128),
  fetched_title TEXT,
  fetched_content TEXT,
  diff_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
  ssrf_check JSONB NOT NULL DEFAULT '{}'::jsonb,
  failure_code VARCHAR(64),
  error_msg TEXT,
  attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  locked_until TIMESTAMPTZ,
  locked_by VARCHAR(128),
  created_by VARCHAR(64) NOT NULL,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  reviewed_by VARCHAR(64),
  reviewed_at TIMESTAMPTZ,
  review_comment TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS wika_url_refresh_schedules (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id),
  knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id),
  source_url TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  cron_expr VARCHAR(128) NOT NULL,
  next_run_at TIMESTAMPTZ NOT NULL,
  last_job_id BIGINT,
  consecutive_failures INT NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
  last_failure_code VARCHAR(64),
  locked_until TIMESTAMPTZ,
  locked_by VARCHAR(128),
  created_by VARCHAR(64) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wika_url_refresh_jobs_knowledge
  ON wika_url_refresh_jobs (tenant_id, kb_id, knowledge_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_wika_url_refresh_jobs_worker
  ON wika_url_refresh_jobs (status, locked_until, created_at)
  WHERE status = 'pending';

CREATE UNIQUE INDEX IF NOT EXISTS ux_wika_url_refresh_jobs_schedule_slot
  ON wika_url_refresh_jobs (schedule_id, scheduled_for)
  WHERE schedule_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_wika_url_refresh_schedules_enabled
  ON wika_url_refresh_schedules (knowledge_id, source_url)
  WHERE enabled = TRUE;

CREATE INDEX IF NOT EXISTS idx_wika_url_refresh_schedules_due
  ON wika_url_refresh_schedules (enabled, next_run_at, locked_until);
