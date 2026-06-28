-- 回滚迁移: 000094_wika_evaluation。

DO $$ BEGIN RAISE NOTICE '[Migration 000094] Dropping eval tables'; END $$;

DROP INDEX IF EXISTS idx_eval_run_items_run;
DROP TABLE IF EXISTS eval_run_items;

DROP INDEX IF EXISTS idx_eval_runs_kb_created;
DROP TABLE IF EXISTS eval_runs;

DROP INDEX IF EXISTS idx_eval_qa_items_dataset_enabled;
DROP TABLE IF EXISTS eval_qa_items;

DROP INDEX IF EXISTS idx_eval_datasets_tenant_kb;
DROP TABLE IF EXISTS eval_datasets;
