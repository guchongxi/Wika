-- Migration: 000103_wika_team_defaults_backfill
-- Description: Backfill default KB and safe suggestion policy for existing team spaces.

DO $$ BEGIN RAISE NOTICE '[Migration 000103] Backfilling team space defaults'; END $$;

WITH team_targets AS (
    SELECT
        t.id AS tenant_id,
        COALESCE(owner_member.user_id, home_user.id) AS created_by
    FROM tenants t
    LEFT JOIN wika_space_defaults wsd
        ON wsd.tenant_id = t.id
    LEFT JOIN LATERAL (
        SELECT tm.user_id
        FROM tenant_members tm
        WHERE tm.tenant_id = t.id
          AND tm.status = 'active'
        ORDER BY
            CASE tm.role
                WHEN 'owner' THEN 1
                WHEN 'admin' THEN 2
                ELSE 3
            END,
            tm.joined_at ASC
        LIMIT 1
    ) owner_member ON TRUE
    LEFT JOIN LATERAL (
        SELECT u.id
        FROM users u
        WHERE u.tenant_id = t.id
          AND u.deleted_at IS NULL
        ORDER BY u.created_at ASC
        LIMIT 1
    ) home_user ON TRUE
    WHERE t.deleted_at IS NULL
      AND t.space_type = 'team'
      AND wsd.tenant_id IS NULL
),
kb_defaults AS (
    SELECT
        COALESCE((SELECT trim(both '"' FROM value::text) FROM system_settings WHERE key = 'kb.default_llm_model_id' LIMIT 1), '') AS llm_model_id,
        COALESCE((SELECT trim(both '"' FROM value::text) FROM system_settings WHERE key = 'kb.default_embedding_model_id' LIMIT 1), '') AS embedding_model_id,
        COALESCE((SELECT trim(both '"' FROM value::text) FROM system_settings WHERE key = 'kb.default_storage_provider' LIMIT 1), 'local') AS storage_provider,
        COALESCE((SELECT (value::text)::int FROM system_settings WHERE key = 'kb.default_chunk_size' LIMIT 1), 512) AS chunk_size,
        COALESCE((SELECT (value::text)::int FROM system_settings WHERE key = 'kb.default_chunk_overlap' LIMIT 1), 80) AS chunk_overlap,
        COALESCE((SELECT value FROM system_settings WHERE key = 'kb.default_chunk_separators' LIMIT 1), '["\n\n", "\n", "。", "！", "？", ";", "；"]'::jsonb) AS separators,
        COALESCE((SELECT (value::text)::boolean FROM system_settings WHERE key = 'kb.default_index_vector_enabled' LIMIT 1), TRUE) AS vector_enabled,
        COALESCE((SELECT (value::text)::boolean FROM system_settings WHERE key = 'kb.default_index_keyword_enabled' LIMIT 1), TRUE) AS keyword_enabled,
        COALESCE((SELECT (value::text)::boolean FROM system_settings WHERE key = 'kb.default_index_wiki_enabled' LIMIT 1), FALSE) AS wiki_enabled,
        COALESCE((SELECT (value::text)::boolean FROM system_settings WHERE key = 'kb.default_index_graph_enabled' LIMIT 1), FALSE) AS graph_enabled
),
inserted_kbs AS (
    INSERT INTO knowledge_bases (
        id,
        name,
        description,
        tenant_id,
        type,
        creator_id,
        embedding_model_id,
        summary_model_id,
        chunking_config,
        image_processing_config,
        cos_config,
        storage_provider_config,
        vlm_config,
        asr_config,
        indexing_strategy,
        created_at,
        updated_at
    )
    SELECT
        uuid_generate_v4()::text,
        '团队知识',
        '团队空间默认知识库',
        team_targets.tenant_id,
        'document',
        team_targets.created_by,
        kb_defaults.embedding_model_id,
        kb_defaults.llm_model_id,
        jsonb_build_object(
            'chunk_size', kb_defaults.chunk_size,
            'chunk_overlap', kb_defaults.chunk_overlap,
            'separators', kb_defaults.separators
        ),
        '{"model_id": "", "enable_multimodal": false}'::jsonb,
        '{}'::jsonb,
        jsonb_build_object('provider', kb_defaults.storage_provider),
        '{}'::jsonb,
        '{}'::jsonb,
        jsonb_build_object(
            'vector_enabled', kb_defaults.vector_enabled,
            'keyword_enabled', kb_defaults.keyword_enabled,
            'wiki_enabled', kb_defaults.wiki_enabled,
            'graph_enabled', kb_defaults.graph_enabled
        ),
        NOW(),
        NOW()
    FROM team_targets
    CROSS JOIN kb_defaults
    WHERE team_targets.created_by IS NOT NULL
    RETURNING id, tenant_id, creator_id
)
INSERT INTO wika_space_defaults (
    tenant_id,
    default_kb_id,
    created_by,
    created_at,
    updated_at
)
SELECT
    tenant_id,
    id,
    creator_id,
    NOW(),
    NOW()
FROM inserted_kbs
ON CONFLICT (tenant_id) DO NOTHING;

INSERT INTO wika_space_policies (
    tenant_id,
    auto_apply_approved,
    policy_version,
    safety_policy,
    created_at,
    updated_at
)
SELECT
    t.id,
    FALSE,
    1,
    '{}'::jsonb,
    NOW(),
    NOW()
FROM tenants t
LEFT JOIN wika_space_policies wsp
    ON wsp.tenant_id = t.id
WHERE t.deleted_at IS NULL
  AND t.space_type = 'team'
  AND wsp.tenant_id IS NULL
ON CONFLICT (tenant_id) DO NOTHING;
