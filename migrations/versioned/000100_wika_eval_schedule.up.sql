CREATE TABLE IF NOT EXISTS wika_eval_schedules (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id),
  dataset_id BIGINT NOT NULL REFERENCES eval_datasets(id),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  cron_expr VARCHAR(128) NOT NULL,
  next_run_at TIMESTAMPTZ NOT NULL,
  last_run_id BIGINT,
  consecutive_failures INT NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
  last_failure_code VARCHAR(64),
  locked_until TIMESTAMPTZ,
  locked_by VARCHAR(128),
  created_by VARCHAR(64) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE eval_runs
  ADD COLUMN IF NOT EXISTS schedule_id BIGINT REFERENCES wika_eval_schedules(id),
  ADD COLUMN IF NOT EXISTS scheduled_for TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS ux_wika_eval_schedules_enabled
  ON wika_eval_schedules (kb_id, dataset_id)
  WHERE enabled = TRUE;

CREATE INDEX IF NOT EXISTS idx_wika_eval_schedules_due
  ON wika_eval_schedules (enabled, next_run_at, locked_until);

CREATE UNIQUE INDEX IF NOT EXISTS ux_eval_runs_schedule_slot
  ON eval_runs (schedule_id, scheduled_for)
  WHERE schedule_id IS NOT NULL;
