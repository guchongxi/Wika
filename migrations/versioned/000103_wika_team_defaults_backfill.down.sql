-- Reverse migration for 000103_wika_team_defaults_backfill.
--
-- This migration intentionally does not delete backfilled team default KBs.
-- Once created, a default KB may contain user data or be referenced by
-- suggestions, evaluations, freshness items, and audit records. Rolling back
-- code should leave those records intact.

DO $$ BEGIN RAISE NOTICE '[Migration 000103] No-op rollback; keeping team defaults'; END $$;
