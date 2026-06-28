DROP INDEX IF EXISTS ux_eval_runs_schedule_slot;

ALTER TABLE eval_runs
  DROP COLUMN IF EXISTS scheduled_for,
  DROP COLUMN IF EXISTS schedule_id;

DROP INDEX IF EXISTS idx_wika_eval_schedules_due;
DROP INDEX IF EXISTS ux_wika_eval_schedules_enabled;
DROP TABLE IF EXISTS wika_eval_schedules;
