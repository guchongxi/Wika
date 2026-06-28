# Wika 技术实现方案

> 版本: 2.0
> 日期: 2026-06-28
> 基于: WeKnora v0.6.2 fork
> 对应需求: [requirements.md](./requirements.md)

## 一、技术目标

Wika 的技术目标是把 WeKnora 已有知识库、RAG、RBAC、MCP、评测和系统设置能力产品化成一条闭环：

```text
MCP/Web 生产 -> 统一入库 -> 权限感知检索 -> 团队沉淀 -> 评测 -> 保鲜
```

本方案不以“新增页面数量”为完成标准，而以可验证闭环为完成标准。

### 1.1 实施读法

本文档面向后端、前端、MCP 和测试实现者。进入编码时按以下顺序读取：

1. 先读 P0 ADR，确认不能反向实现的边界。
2. 再读对应阶段的数据模型、服务设计、API 契约和安全设计。
3. 最后按“分期实施”和“完成门禁”拆 PR。

每个阶段的最小交付单元必须同时包含：

- migration up/down 或明确说明本阶段不需要 migration。
- service/repository 层单元测试。
- handler/API 测试。
- 权限和越权测试。
- MCP 或前端真实入口测试，按阶段需要选择。
- 阶段验收数据，能复现 `requirements.md` 的最小验证路径。

所有实现继续遵循 TDD：先提交或至少先运行失败测试，再写最小实现，最后补重构和回归。没有失败测试的阶段不能进入“完成”状态。

## 二、当前代码基础

已确认可复用能力：

- Tenant + `tenant_members` + RBAC 已具备空间隔离基础。
- `KnowledgeBase` 已有 `TenantID` 和 `CreatorID`。
- `Knowledge` 已有 `Type`、`Title`、`Source`、`Channel`、`Metadata`、`ParseStatus`。
- 手动知识已有 `ManualKnowledgePayload` 和创建链路。
- 检索已有关键词、向量、混合检索和 chat pipeline。
- 系统设置已有 `system_settings`，支持 DB > ENV > default 的解析模式。
- MCP server 已支持 stdio、SSE、HTTP，但当前工具偏管理型。
- 评测已有 metric 包，但需要持久化 run 和 case 明细。
- Neo4j/图谱能力已有基础，但不作为 MVP 强依赖。

必须修正的旧方案：

- 不给 `knowledge_bases` 新增 `owner_id`。
- 不用 `knowledge_bases.visibility` 表达个人/团队。
- 不恢复 `knowledge_bases.is_pinned` 落库，当前 pin 已是用户维度。
- 不让日常 MCP 工具使用租户级 API key。
- 不把评测结果只存在内存或只存宽 JSON。
- 不在检索热路径中频繁更新 `knowledges` 主表。

## 三、P0 ADR 决策

这些决策是进入编码前的前置条件，后续章节均按这些决策设计。

| ADR | 决策 | 原因 |
|-----|------|------|
| ADR-01 | Space 复用 Tenant，`tenants.space_type` 表达 `personal/team` | 复用现有 tenant_members、RBAC、审计和上下文切换能力 |
| ADR-02 | 一人一个个人空间的不变量由 `user_personal_spaces` 唯一表承载 | 减少直接在上游 `tenants` 表上添加复杂唯一约束的合并风险 |
| ADR-03 | 个人/团队边界不通过 `knowledge_bases.visibility` 表达 | 避免 KB 级可见性与 Tenant/RBAC 双重语义冲突 |
| ADR-04 | 日常 MCP 工具使用用户级 PAT，OAuth 后置 | 不能用租户 API key 伪装 Admin 写入个人知识 |
| ADR-05 | `search_knowledge` 使用 scope-aware fan-out hybrid search | 先解析用户可访问 scope，再复用现有 KB hybrid-search 能力 |
| ADR-06 | `suggest_to_team` 默认不自动发布到团队 | 默认进入可应用结果；只有团队策略开启且确定性安全门禁通过才自动应用 |
| ADR-07 | 旧 KB/knowledge/search/download/preview API 必须接入 personal scope | 只保护 Wika 新 API 会留下 IDOR |
| ADR-08 | P5 Governance 使用独立读写模型，写入仍走现有知识链路 | 冲突、版本、URL 重抓等治理能力不能绕过知识更新、索引和审计 |
| ADR-09 | P5 异步任务统一使用 DB lease + 幂等 worker | 避免多实例重复触发 URL 重抓、评测计划和冲突检测 |
| ADR-10 | Organization 共享默认 reference + allowed_fields + ScopeResolver shared scope | 跨团队共享不能复制正文，也不能绕过接收团队权限 |

### 3.1 ADR 详述

#### ADR-01 Space 复用 Tenant

- **背景**：WeKnora 已经用 Tenant、`tenant_members`、RBAC 和审计承载多租户边界。Wika 需要个人/团队空间，但不需要再造一套资源归属体系。
- **备选方案**：
  - 新建 `spaces` 表并映射到 tenant：语义干净，但所有现有 KB、Agent、评测、审计都要双写或 join，合并上游风险高。
  - 复用 Tenant 并新增 `space_type`：侵入点少，能直接复用现有权限链，但需要给旧 API 补 personal scope。
- **决策**：复用 Tenant，新增 `tenants.space_type`，取值 `personal/team`。
- **影响**：所有空间切换本质仍是 tenant 切换；代码和 UI 统一展示为 Space。旧 tenant 默认迁移为 `team`。
- **回滚/迁移**：`space_type` 默认 `team`，回滚时可忽略 personal 相关表；已创建 personal tenant 不删除，只停止入口创建。

#### ADR-02 一人一个个人空间由 `user_personal_spaces` 承载

- **背景**：需要强约束“一人一个个人空间”和“一个 personal tenant 只属于一个用户”。
- **备选方案**：
  - 在 `tenants` 增加 `owner_user_id` 并做复杂唯一约束：查询直接，但改上游核心表更多。
  - 独立映射表：约束清晰，合并冲突低。
- **决策**：新增 `user_personal_spaces(user_id, tenant_id)`，二者均唯一。
- **影响**：注册和首次访问必须在事务内创建 tenant、tenant_members Owner、默认 KB、映射记录。
- **回滚/迁移**：不自动删除个人空间数据；回滚入口只停止创建和展示。

#### ADR-03 个人/团队边界不使用 KB visibility

- **背景**：KB visibility 表达知识库可见性，不适合表达个人空间身份。用它表达个人知识会和 Tenant/RBAC 出现双重权限语义。
- **决策**：个人/团队是 Space 属性；KB 只属于某个 Space。
- **影响**：所有 KB、knowledge、search、download、preview 必须先解析 space scope。

#### ADR-04 日常 MCP 使用用户级 PAT，OAuth 后置

- **背景**：租户 API key 面向管理型集成，不能代表某个用户写入个人知识。
- **决策**：P1-P5 当前只实现用户级 PAT，绑定 user、默认 tenant、scope、过期和撤销状态；日常 MCP tools 拒绝租户 API key。OAuth 作为后续同等身份形态，只有补齐 PKCE、refresh、scope 映射和撤销策略后才启用。
- **影响**：MCP server 需区分管理型 tools 和日常 tools；审计日志记录真实 user_id。
- **回滚/迁移**：保留原管理型 API key；日常 tools 可整体关闭。

#### ADR-05 `search_knowledge` 使用 scope-aware fan-out hybrid search

- **背景**：agentmemory 的 `search_knowledge` 可借鉴 BM25、向量、图谱、RRF 和 compact 返回，但 Wika 的第一约束是空间权限。
- **决策**：先用 ScopeResolver 得到可读 KB 集合，再并发调用现有单 KB hybrid search，最后跨 KB RRF 合并和 rerank。
- **影响**：底层检索引擎 P1 不重写；Wika 层负责跨空间编排、权限过滤、访问记录和 AI 安全边界。

#### ADR-06 `suggest_to_team` 默认不自动发布

- **背景**：个人知识可能包含临时经验、敏感内容或低质量内容，直接进入团队会污染团队资产。
- **决策**：AI 预审输出三态；团队策略默认关闭自动应用。只有策略开启、AI 通过、确定性安全门禁通过时才由系统自动应用。
- **影响**：人工仍可覆盖所有结果；自动应用必须可审计、可追溯、可回滚。

#### ADR-07 旧 API 必须接入 personal scope

- **背景**：只保护 Wika 新 API 会留下旧 WeKnora API 的 IDOR 通道。
- **决策**：KB/knowledge/search/download/preview/hybrid-search 等旧接口必须接入统一 ScopeResolver。
- **影响**：这是 P1a 安全门禁，不通过不得进入 P1b/P1c。

#### ADR-08 P5 Governance 使用独立读写模型，写入仍走现有知识链路

- **背景**：P5 的冲突、版本、URL 重抓和跨团队共享都需要自己的状态机，但最终影响的是已有 `knowledges`、chunk、embedding 和审计链路。
- **决策**：P5 新增独立治理表承载候选、版本、任务和共享授权；任何会修改知识正文、标题、标签或状态的动作必须调用现有知识更新链路，并触发索引和版本记录。
- **影响**：ConflictService 只能生成候选；URLRefreshService 只能在人工确认后调用知识更新；VersionService 的 restore 本身也生成新版本。
- **回滚/迁移**：关闭 P5 功能开关后，治理表保留为只读，不删除历史审计和 lineage。

#### ADR-09 P5 异步任务统一使用 DB lease + 幂等 worker

- **背景**：URL 重抓、定时评测和冲突检测会由 worker 执行，多实例部署时容易重复触发或无限重试。
- **决策**：P5 worker 必须通过数据库锁、lease 或等价机制领取任务；任务状态、`attempts`、`next_run_at`、`locked_until`、`locked_by` 和幂等键由 repository 事务维护。
- **影响**：worker crash 后任务可被重新领取；连续失败达到阈值后停用或降频；重复调用 apply/review/revoke 返回已有终态或 409。
- **回滚/迁移**：可单独停用 worker，不影响人工查看已生成的治理项。

#### ADR-10 Organization 共享默认 reference + allowed_fields + ScopeResolver shared scope

- **背景**：Organization 跨团队共享如果复制正文，会扩大数据爆炸半径；如果直接把源 KB 暴露给接收团队，会绕过来源团队治理。
- **决策**：P5 默认只做 `reference` 共享，`allowed_fields` 服务端白名单裁剪，active share 通过 ScopeResolver 输出 shared scope；撤销后 resolver 不再返回该 scope。
- **影响**：SearchService 命中 shared scope 后仍必须按 `allowed_fields` 裁剪；个人 KB 默认禁止 Organization share；SystemAdmin 不因 share 获得正文读取权。
- **回滚/迁移**：停用 shared scope 后，历史访问日志和 lineage metadata 保留，新搜索不再命中共享内容。

## 四、总体架构

保持模块化单体，不拆微服务。

新增 Wika 编排层：

```text
internal/wika/
  space/          # Space 门面，封装 Tenant/tenant_members
  auth/           # 用户级 MCP PAT scope，OAuth 后置映射到同一 scope
  intake/         # MCP/Web 统一知识生产入口
  search/         # scope-aware hybrid search
  suggestion/     # suggest_to_team AI 预审与人工覆盖
  evaluation/     # 评测数据集、run、case 明细
  freshness/      # 保鲜状态、访问聚合、扫描任务
  admin/          # 默认配置、空间概览、系统治理
  graph/          # 图谱读模型，后置增强
```

原则：

- `internal/wika/**` 只做门面和编排，不复制 WeKnora 领域服务。
- 必须接入旧接口的权限路径时，可以修改现有 service/repository，但要保持窄改。
- 不随意给 Go 接口追加方法；Go 接口新增方法会破坏所有实现，属于高风险改动。
- 迁移编号使用 `000090+`。

## 五、数据模型

### 5.1 空间模型

新增字段：

```text
tenants.space_type: personal | team
```

新增表：

```text
user_personal_spaces
  user_id
  tenant_id
  created_at
```

约束：

- `tenants.space_type` 默认 `team`，历史租户迁移为 `team`。
- `user_personal_spaces.user_id` 唯一。
- `user_personal_spaces.tenant_id` 唯一。
- 表内 `tenant_id` 必须指向 `space_type=personal` 的 tenant。
- personal tenant 只允许一个 Owner 成员。
- team tenant 继续复用 `tenant_members` 和 RBAC。
- 注册或首次访问时通过事务创建 personal tenant、tenant_members Owner 记录和 user_personal_spaces 记录。

### 5.2 默认知识库

新增：

```text
wika_space_defaults
  id
  tenant_id
  default_kb_id
  created_by
  created_at
  updated_at
```

用途：

- `push_knowledge` 默认写入个人空间默认 KB。
- 团队审批通过后默认复制到目标团队默认 KB。

约束：

- `tenant_id` 唯一。
- `default_kb_id` 必须属于同一 `tenant_id`。
- 个人空间首次创建时自动创建默认 KB 并写入本表。

### 5.3 空间策略

新增：

```text
wika_space_policies
  id
  tenant_id
  auto_apply_approved
  safety_policy jsonb
  created_at
  updated_at
```

用途：

- 保存团队空间的 `suggest_to_team` 自动应用策略。
- P1 默认 `auto_apply_approved=false`。
- `safety_policy` 保存敏感词、AI 修正幅度阈值、重复风险阈值等团队策略。

约束：

- `tenant_id` 唯一。
- personal 空间可以没有策略记录。
- 自动应用时必须把策略快照写入 `knowledge_suggestions.policy_version/ai_review`。

### 5.4 知识状态扩展

不在 MVP 里给 `knowledges` 堆大量治理字段。新增：

```text
wika_knowledge_state
  knowledge_id
  tenant_id
  kb_id
  quality_score
  quality_breakdown jsonb
  freshness_status
  confidence_score
  expires_at
  source_hash
  source_updated_at
  idempotency_key
  last_access_rollup_at
  review_status
  created_at
  updated_at
```

索引：

- `(tenant_id, kb_id, freshness_status)`
- `(kb_id, expires_at) WHERE expires_at IS NOT NULL`
- `(knowledge_id)` unique
- `(tenant_id, kb_id, idempotency_key) WHERE idempotency_key IS NOT NULL`

P1 必须落地最小字段：

- `knowledge_id`
- `tenant_id`
- `kb_id`
- `quality_score`
- `quality_breakdown`
- `freshness_status`
- `expires_at`
- `source_hash`
- `idempotency_key`
- `review_status`

P3 再扩展扫描和访问汇总字段的完整使用。

### 5.5 访问聚合

检索命中不直接更新 `knowledges`。

新增日聚合：

```text
knowledge_access_daily
  tenant_id
  kb_id
  knowledge_id
  day
  access_count
  last_accessed_at
```

唯一约束：

- `(tenant_id, kb_id, knowledge_id, day)`

用途：

- 复用统计。
- 保鲜扫描。
- 搜索质量分析。

写入策略：

- 搜索热路径只投递轻量事件或批量 upsert，不同步更新 `knowledges`。
- upsert 冲突键为 `(tenant_id, kb_id, knowledge_id, day)`。
- 访问写入失败不影响搜索返回，但必须记录日志和指标。

### 5.6 团队推荐与溯源

新增：

```text
knowledge_suggestions
  id
  source_tenant_id
  source_kb_id
  source_knowledge_id
  target_tenant_id
  target_kb_id
  submitter_id
  idempotency_key
  source_content_hash
  reason
  ai_decision                 # approved | needs_confirmation | rejected
  ai_confidence
  ai_review jsonb
  corrected_title
  corrected_content
  corrected_tags jsonb
  change_summary jsonb
  human_decision              # approved | needs_confirmation | rejected | null
  human_reviewer_id
  human_comment
  final_decision
  status                      # ai_reviewed | pending_human | applied | rejected | cancelled
  auto_apply_enabled
  policy_version
  result_knowledge_id
  created_at
  reviewed_at
  applied_at
```

新增：

```text
knowledge_lineage
  id
  source_knowledge_id
  target_knowledge_id
  source_tenant_id
  target_tenant_id
  mode                        # copy | reference
  suggestion_id
  created_by
  created_at
```

MVP 默认 `mode=copy`。KB 级引用共享后置复用 Organization。

约束：

- 同一 `source_knowledge_id + target_tenant_id + target_kb_id` 同时只能存在一个未终态 suggestion。
- `idempotency_key` 非空时，同一提交者、目标团队和幂等键必须唯一。
- `result_knowledge_id` 必须属于 `target_tenant_id/target_kb_id`。
- `auto_apply_enabled` 记录提交时团队策略快照，不能只读取当前团队配置。
- 终态为 `applied/rejected/cancelled`；终态记录不得被重新应用。

### 5.7 评测

新增：

```text
eval_datasets
  id
  tenant_id
  kb_id
  name
  description
  created_by
  created_at
  updated_at

eval_qa_items
  id
  dataset_id
  question
  expected_answer
  expected_knowledge_ids jsonb
  expected_chunk_ids jsonb
  tags jsonb
  enabled
  version
  created_at
  updated_at

eval_runs
  id
  tenant_id
  kb_id
  dataset_id
  dataset_version
  trigger
  status
  mrr
  recall_at_5
  ndcg_at_5
  metrics jsonb
  search_config jsonb
  total
  failed
  error_msg
  created_by
  started_at
  completed_at

eval_run_items
  id
  run_id
  qa_item_id
  rank
  score
  hit
  first_hit_rank
  retrieved_chunk_ids jsonb
  retrieved_knowledge_ids jsonb
  failure_reason
  error_msg
  created_at
```

指标口径：

- `Recall@5`：任一 `expected_chunk_ids` 或 `expected_knowledge_ids` 出现在前 5 即命中。
- `MRR`：取第一个正确命中的倒数排名；无命中为 0。
- `NDCG@5`：P1 可按二值相关性计算；多级相关性后置。
- QA 未配置期望 chunk/knowledge 时，只允许 dry-run，不计入正式指标。
- 每次 run 必须记录检索参数、模型版本和索引策略快照。

### 5.8 保鲜扫描

新增：

```text
freshness_checks
  id
  tenant_id
  kb_id
  trigger
  status
  checked_at
  completed_at

freshness_check_items
  id
  check_id
  knowledge_id
  issue_type                  # expired | expiring | stale | low_quality | low_confidence
  severity                    # low | medium | high
  suggested_action
  status                      # open | resolved | ignored
  resolution_action           # mark_updated | extend_expiry | deprecate | ignore | resuggest_to_team
  resolution_note
  previous_status
  resolved_by
  resolved_at
  created_at
  updated_at
```

不要把扫描明细塞进一个大 JSON 数组。

### 5.9 DDL 级约束补充

以下约束必须在 migration、repository 事务和测试中同时体现。数据库不能表达的跨表不变量，必须由 service 事务和回归测试兜底。

#### `tenants.space_type`

```sql
ALTER TABLE tenants
  ADD COLUMN space_type varchar(16) NOT NULL DEFAULT 'team',
  ADD CONSTRAINT chk_tenants_space_type CHECK (space_type IN ('personal', 'team'));

CREATE INDEX idx_tenants_space_type ON tenants(space_type);
```

迁移策略：

- 历史 tenant 全部保持默认 `team`。
- personal tenant 只通过 `SpaceService.GetOrCreatePersonalSpace` 创建。
- 回滚时先删除 CHECK 和索引，再删除字段；不自动删除 tenant 数据。

#### `user_personal_spaces`

```sql
CREATE TABLE user_personal_spaces (
  user_id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_user_personal_spaces_tenant_id ON user_personal_spaces(tenant_id);
```

跨表不变量：

- `tenant_id` 指向的 tenant 必须是 `space_type='personal'`。
- personal tenant 在 `tenant_members` 中只能有一个 Owner，且必须是 `user_id`。
- 该不变量由创建事务、成员变更拦截和测试保证；如后续需要更强保护，可加 trigger。

#### `wika_space_defaults`

```sql
CREATE TABLE wika_space_defaults (
  id bigserial PRIMARY KEY,
  tenant_id varchar(64) NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
  default_kb_id varchar(64) NOT NULL REFERENCES knowledge_bases(id) ON DELETE RESTRICT,
  created_by varchar(64) NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_wika_space_defaults_default_kb_id ON wika_space_defaults(default_kb_id);
```

同租户约束：

- `default_kb_id` 必须属于同一 `tenant_id`。
- PostgreSQL 不能用普通 FK 表达 `knowledge_bases(id, tenant_id)` 时，由 `SetDefaultKB` 事务 `SELECT ... FOR UPDATE` 校验，并用 repository 测试覆盖跨 tenant 拒绝。

#### `wika_knowledge_state`

```sql
CREATE TABLE wika_knowledge_state (
  knowledge_id varchar(64) PRIMARY KEY REFERENCES knowledges(id) ON DELETE CASCADE,
  tenant_id varchar(64) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  kb_id varchar(64) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
  quality_score int NOT NULL DEFAULT 0 CHECK (quality_score BETWEEN 0 AND 100),
  quality_breakdown jsonb NOT NULL DEFAULT '{}'::jsonb,
  freshness_status varchar(24) NOT NULL DEFAULT 'fresh'
    CHECK (freshness_status IN ('fresh', 'expiring', 'expired', 'stale', 'low_quality', 'needs_review')),
  confidence_score numeric(4,3) CHECK (confidence_score IS NULL OR confidence_score BETWEEN 0 AND 1),
  expires_at timestamptz,
  source_hash varchar(128),
  source_updated_at timestamptz,
  idempotency_key varchar(128),
  last_access_rollup_at timestamptz,
  review_status varchar(24) NOT NULL DEFAULT 'none'
    CHECK (review_status IN ('none', 'needs_review', 'reviewed', 'deprecated')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_wika_knowledge_state_tenant_kb_freshness
  ON wika_knowledge_state(tenant_id, kb_id, freshness_status);
CREATE INDEX idx_wika_knowledge_state_kb_expires
  ON wika_knowledge_state(kb_id, expires_at)
  WHERE expires_at IS NOT NULL;
CREATE UNIQUE INDEX ux_wika_knowledge_state_idempotency
  ON wika_knowledge_state(tenant_id, kb_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;
```

#### `knowledge_access_daily`

```sql
CREATE TABLE knowledge_access_daily (
  tenant_id varchar(64) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  kb_id varchar(64) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
  knowledge_id varchar(64) NOT NULL REFERENCES knowledges(id) ON DELETE CASCADE,
  day date NOT NULL,
  access_count bigint NOT NULL DEFAULT 0 CHECK (access_count >= 0),
  last_accessed_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, kb_id, knowledge_id, day)
);

CREATE INDEX idx_knowledge_access_daily_kb_day
  ON knowledge_access_daily(kb_id, day DESC);
CREATE INDEX idx_knowledge_access_daily_knowledge_day
  ON knowledge_access_daily(knowledge_id, day DESC);
```

写入策略：

- 搜索热路径只写入内存队列、Redis stream 或批量 buffer。
- flush worker 每 1-5 秒批量 upsert，单批建议 100-1000 条。
- upsert 只更新 `access_count = access_count + excluded.access_count` 和 `last_accessed_at = greatest(...)`。
- flush 失败记录指标和日志，不影响搜索响应。

#### `knowledge_suggestions`

必须补充：

```sql
CREATE UNIQUE INDEX ux_knowledge_suggestions_open
  ON knowledge_suggestions(source_knowledge_id, target_tenant_id, target_kb_id)
  WHERE status IN ('ai_reviewed', 'pending_human');

CREATE UNIQUE INDEX ux_knowledge_suggestions_idempotency
  ON knowledge_suggestions(submitter_id, target_tenant_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;
```

事务要求：

- `Apply` 使用 `SELECT ... FOR UPDATE` 锁定 suggestion。
- 应用前重算 `source_content_hash`，变化则降级为 `pending_human`。
- 复制知识、写 `knowledge_lineage`、写 audit log 必须在同一事务或可补偿工作流内完成。

#### `wika_user_tokens`

```sql
CREATE TABLE wika_user_tokens (
  id bigserial PRIMARY KEY,
  user_id varchar(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  tenant_id varchar(64) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name varchar(128) NOT NULL,
  token_prefix varchar(32) NOT NULL,
  token_hash varchar(128) NOT NULL UNIQUE,
  hash_alg varchar(32) NOT NULL DEFAULT 'sha256_pepper',
  scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by_ip inet,
  last_used_at timestamptz
);

CREATE INDEX idx_wika_user_tokens_user_tenant_active
  ON wika_user_tokens(user_id, tenant_id)
  WHERE revoked_at IS NULL;
CREATE INDEX idx_wika_user_tokens_expires_at ON wika_user_tokens(expires_at);
```

安全要求：

- token 明文只在创建响应返回一次。
- 日志、审计、错误响应只允许出现 `token_prefix`。
- 创建、撤销、列表接口必须限流；token 校验失败不返回 token 是否存在。

### 5.10 P4 图谱读模型

P4 不要求重写 Neo4j 抽取链路，但需要在关系库保留可分页、可鉴权、可降级的读模型。

新增：

```text
wika_graph_entities
  id
  tenant_id
  kb_id
  entity_key
  name
  entity_type
  summary
  source_knowledge_ids jsonb
  confidence_score
  last_extracted_at
  created_at
  updated_at

wika_graph_edges
  id
  tenant_id
  kb_id
  source_entity_id
  target_entity_id
  relation_type
  evidence_knowledge_id
  evidence_chunk_id
  evidence_text
  confidence_score
  created_at
  updated_at
```

索引：

- `wika_graph_entities(tenant_id, kb_id, entity_type, name)`
- `wika_graph_entities(tenant_id, kb_id, entity_key)` unique
- `wika_graph_edges(tenant_id, kb_id, source_entity_id)`
- `wika_graph_edges(tenant_id, kb_id, target_entity_id)`
- `wika_graph_edges(evidence_knowledge_id)`

权限：

- API 返回前必须用 KB scope 过滤。
- SystemAdmin 不返回 `summary` 和 `evidence_text`，只返回统计字段。
- 图谱增强检索只把 allowed KB 内的实体/边加入召回。

图谱检索配置落点：

- 开关、权重和降级策略进入 `wika_space_policies.safety_policy.search.graph`，P4 默认 `enabled=false`、`weight=0.15`、`fail_open=true`。
- SystemAdmin 可以在系统默认设置里给新团队提供只读默认值，但运行时以团队 policy 为准。
- SearchService 每次返回 `graph_contribution` 和 `graph_degraded`，用于判断开启后是否影响主搜索。

### 5.11 P5 高级治理模型

#### 冲突检测

```text
wika_conflict_checks
  id
  tenant_id
  kb_id
  trigger
  status
  attempts
  locked_until
  locked_by
  created_by
  started_at
  completed_at
  error_msg

wika_conflict_items
  id
  check_id
  tenant_id
  kb_id
  source_knowledge_id
  target_knowledge_id
  conflict_type              # contradiction | duplicate | outdated | scope_overlap
  confidence_score
  evidence jsonb
  ai_explanation
  status                     # open | confirmed | dismissed | resolved
  resolved_by
  resolved_at
  created_at
  updated_at
```

约束：

- 同一 `source_knowledge_id + target_knowledge_id + conflict_type` 未终态只能存在一条。
- `evidence` 只存片段定位和 hash；API 返回证据正文前重新做 scope。

#### 知识版本

```text
wika_knowledge_versions
  id
  knowledge_id
  tenant_id
  kb_id
  version_no
  title
  content
  tags jsonb
  status
  review_status
  metadata jsonb
  content_hash
  change_reason
  created_by
  created_at
```

约束：

- `(knowledge_id, version_no)` unique。
- 恢复旧版本时创建新的 `version_no`，不修改历史版本。
- 版本内容按 knowledge read scope 控制；SystemAdmin 不可读取个人版本正文。
- `status` 和 `review_status` 记录知识关键状态快照；其他非核心字段进入 `metadata`。

#### URL 重抓

```text
wika_url_refresh_jobs
  id
  schedule_id
  knowledge_id
  tenant_id
  kb_id
  source_url
  status                     # pending | running | pending_review | applied | rejected | failed
  fetched_hash
  fetched_title
  fetched_content
  diff_summary jsonb
  ssrf_check jsonb
  error_msg
  attempts
  locked_until
  locked_by
  created_by
  started_at
  completed_at
  reviewed_by
  reviewed_at
  created_at

wika_url_refresh_schedules
  id
  tenant_id
  kb_id
  knowledge_id
  source_url
  enabled
  cron_expr
  next_run_at
  last_job_id
  consecutive_failures
  created_by
  created_at
  updated_at
```

约束：

- 只允许 HTTP/HTTPS。
- 禁止 private、loopback、link-local、multicast、云元数据地址和重定向后的禁用地址。
- 限制响应大小、超时、content-type；抓取失败不改变原知识。
- `pending_review` 经人工确认后才写入知识并生成新版本。
- schedule 只负责生成 job，不直接抓取或写知识。
- 同一 `knowledge_id + source_url` 最多一个启用 schedule。
- worker 使用 `locked_until/locked_by` 领取 job；连续失败达到阈值后停用 schedule 或延长 `next_run_at`。

#### 定时评测

```text
wika_eval_schedules
  id
  tenant_id
  kb_id
  dataset_id
  cron_expr
  enabled
  next_run_at
  last_run_id
  consecutive_failures
  created_by
  created_at
  updated_at
```

约束：

- 同一 `kb_id + dataset_id` 最多一个启用计划。
- 连续失败达到阈值后自动停用或降频，并写 audit log。

#### Organization 共享

```text
wika_organizations
  id
  name
  created_by
  created_at
  updated_at

wika_org_members
  org_id
  tenant_id
  role                       # owner | admin | member
  created_at

wika_org_shares
  id
  org_id
  source_tenant_id
  source_kb_id
  target_tenant_id
  mode                       # reference | snapshot
  allowed_fields jsonb
  status                     # active | revoked
  created_by
  revoked_by
  revoked_at
  created_at
```

约束：

- P5 默认 `mode=reference`，不复制正文。
- `allowed_fields` 默认不含个人正文和证据；共享搜索命中后仍要校验接收团队成员权限。
- revoke 后新 ScopeResolver 不再返回该 share scope。

## 六、核心服务设计

### 6.1 SpaceService

职责：

- 获取当前用户个人空间。
- 注册时创建个人空间。
- 创建团队空间。
- 列出用户可访问空间。
- 校验 personal/team 不变量。

关键方法：

```text
GetPersonalSpace(ctx, userID)
GetOrCreatePersonalSpace(ctx, userID)
CreateTeamSpace(ctx, ownerID, payload)
ListUserSpaces(ctx, userID)
AssertSpaceAccess(ctx, userID, tenantID, requiredRole)
```

### 6.2 IntakeService

统一入库链路：

```text
KnowledgeDraft
  -> Normalize
  -> Score
  -> DuplicateCheck
  -> Persist
  -> Index
```

MVP 实现：

- Web 和 MCP 共用同一个 service。
- 默认写入个人空间默认 KB。
- 调用现有手动知识创建链路。
- 将质量分、修正结果、来源、幂等键写入 `wika_knowledge_state` 和 `knowledge.metadata`。

降级：

- LLM normalize 失败时，用原文入库，标记 `needs_review`。
- embedding 失败时，保留知识并允许重试。

### 6.3 SearchService

目标：

让 `search_knowledge` 成为 AI 友好的权限感知混合检索。

流程：

```text
ResolveSearchScope(user)
  -> BuildQueryPlan(query)
  -> FanOutHybridSearch(kb scopes)
  -> OptionalQueryExpansion
  -> OptionalGraphSearch
  -> CrossScopeRRFMerge
  -> QualityFreshnessRerank
  -> CompactResponse
  -> RecordAccess
```

借鉴 agentmemory：

- BM25 + vector + graph 三路召回。
- RRF 合并排名。
- query expansion 提升召回。
- compact/expand 两段式响应。
- graph 检索 best-effort。
- 检索命中记录访问。

Wika 差异：

- 必须先做租户/空间权限 scope。
- 结果需要标明 personal/team/shared 来源。
- 排序需要考虑质量分和保鲜状态。
- 返回给 AI 的内容必须带“不可信资料”边界。

实现映射：

- `ResolveSearchScope` 返回当前用户个人空间默认 KB、已加入团队可读 KB、共享 KB 三类 scope。
- P1 不新增底层检索引擎，优先复用现有 `KnowledgeBaseService.HybridSearch(ctx, kbID, params)`。
- 对多个 KB 并发 fan-out 调用单 KB hybrid search，每个 KB 内部仍由现有关键词 + 向量 + RRF 完成。
- Wika 层对所有 KB 的结果做二次 RRF，合并维度为 `knowledge_id/chunk_id`。
- P1 `query expansion` 默认关闭；只在团队配置开启时最多扩展 3 个 query，并计入成本指标。
- P1 图谱检索 best-effort，失败只记录降级指标，不影响主搜索。
- `QualityFreshnessRerank` 只读取 `wika_knowledge_state`，不得回写主表。
- `RecordAccess` 通过 `knowledge_access_daily` 异步或批量 upsert；失败不影响响应。
- `expand_knowledge_result` 必须重新做 scope 校验，不能信任 compact 结果中的 ID。

MVP MCP 工具：

```text
search_knowledge(query, limit=5, include_team=true, format=compact)
expand_knowledge_result(ids[])
```

如果不想新增第二个 MCP 工具，也可以让 `search_knowledge` 支持 `format=full`，但默认必须是 compact。

### 6.3.1 ScopeResolver 契约

ScopeResolver 是 Wika 权限边界的唯一入口。Wika 新 API 和旧 WeKnora API 都必须调用它，不能在 handler 内各自拼权限条件。

输入：

```text
actor:
  user_id
  is_system_admin
  auth_type              # jwt | user_pat | tenant_api_key | embed_token
  default_tenant_id

resource:
  kind                   # tenant | kb | knowledge | chunk | file | suggestion | eval | freshness | graph | conflict_check | conflict_item | knowledge_version | url_refresh_job | eval_schedule | org | org_share
  id
  parent_id?

action:
  read | write | search | download | preview | review | apply | admin | metadata_read

options:
  include_team
  include_shared
  allow_metadata_only
  batch_mode
```

输出：

```text
decision:
  allowed: bool
  not_found: bool
  metadata_only: bool
  scopes:
    - tenant_id
      kb_id?
      source: personal | team | shared | organization
      role
      allowed_fields
  denial_reason_for_log
```

语义：

- 无权限访问个人资源时，对外返回 404；日志记录 `denial_reason_for_log`。
- batch 查询逐条过滤，不能因为部分无权整体 403，也不能返回无权 ID 的占位错误。
- search 查询只返回 allowed scopes；无 scope 返回空结果。
- SystemAdmin 默认只能 `metadata_read`，`allowed_fields` 使用字段级 allowlist。
- `tenant_api_key` 不允许执行日常 personal action，例如 `knowledge:push/search/read/suggestion:create`。
- `expand_knowledge_result`、download、preview 必须重新解析资源 scope，不能信任搜索 compact 结果。
- P5 direct-id 资源必须先回溯父 KB 或 knowledge，再按父资源执行 scope，不能只校验 ID 存在。
- Organization shared scope 只由 active share 产生，且必须带 `allowed_fields`；revoke 后新 resolver 结果不得包含该 scope。

P5 direct-id 回溯规则：

| resource kind | 回溯路径 | 允许动作 | 无权响应 |
|---------------|----------|----------|----------|
| `conflict_check` | check -> kb_id -> tenant_id | read/admin | 404 |
| `conflict_item` | item -> check/kb_id -> tenant_id | read/review/admin | 404 |
| `knowledge_version` | version -> knowledge_id -> kb_id -> tenant_id | read/restore | 404 |
| `url_refresh_job` | job -> knowledge_id -> kb_id -> tenant_id | read/review/apply/admin | 404 |
| `eval_schedule` | schedule -> kb_id/dataset_id -> tenant_id | read/admin | 404 |
| `org` | org -> org_members.tenant_id + actor tenant role | read/admin | 403 或 404，按是否会暴露个人资源决定 |
| `org_share` | share -> source_kb_id + target_tenant_id + org membership | read/admin/search | 404 |

旧 API 接入点：

| 区域 | 接入方式 | P1a 测试 |
|------|----------|----------|
| KB 列表/详情 | 列表按 `ListReadableKBScopes` 过滤；详情按 `Resolve(kb, read)` | A 看不到 B personal KB |
| HybridSearch | 请求 KB 前 `Resolve(kb, search)` | A 搜 B personal KB 返回 404 或空 |
| Knowledge 详情 | `Resolve(knowledge, read)` 需回溯 KB/tenant | A 读 B personal knowledge 返回 404 |
| Knowledge batch | 对每个 ID 调 resolver 后过滤 | batch 不泄露 B 的 ID 存在 |
| Knowledge search | 先解析 readable scopes，再拼查询 | 不跨 personal space |
| download/preview | 按 knowledge 回溯 KB scope | 文件内容不泄露 |
| eval/freshness/graph | 按 KB 和字段 allowlist 校验 | SystemAdmin 不读正文 |

旧 API 精确 inventory：

| method/path | handler/service 接入点 | resource/action | 无权语义 | P1a RED 测试 |
|-------------|------------------------|-----------------|----------|--------------|
| `GET /api/v1/knowledge-bases` | `KnowledgeBaseHandler.ListKnowledgeBases` | readable KB scopes | 过滤无权 KB | `TestWikaScopeFiltersPersonalKBList` |
| `GET /api/v1/knowledge-bases/:id` | `KnowledgeBaseHandler.GetKnowledgeBase` | `kb/read` | personal 越权 404 | `TestWikaScopeRejectsPersonalKBDetail` |
| `GET/POST /api/v1/knowledge-bases/:id/hybrid-search` | `KnowledgeBaseHandler.HybridSearch` | `kb/search` | 404 或空结果，不返回 snippet | `TestWikaScopeRejectsHybridSearch` |
| `GET /api/v1/knowledge-bases/:id/knowledge` | `KnowledgeHandler.ListKnowledge` | `kb/read` | 404 或空列表 | `TestWikaScopeRejectsKnowledgeList` |
| `POST /api/v1/knowledge-bases/:id/knowledge/manual` | `KnowledgeHandler.CreateManualKnowledge` | `kb/write` | 404 或 403，personal 只允许 Owner | `TestWikaScopeRejectsManualCreateIntoOtherPersonal` |
| `POST /api/v1/knowledge-bases/:id/knowledge/file` | `KnowledgeHandler.CreateFileKnowledge` | `kb/write` | 404 或 403 | `TestWikaScopeRejectsFileCreateIntoOtherPersonal` |
| `POST /api/v1/knowledge-bases/:id/knowledge/url` | `KnowledgeHandler.CreateURLKnowledge` | `kb/write` | 404 或 403 | `TestWikaScopeRejectsURLCreateIntoOtherPersonal` |
| `GET /api/v1/knowledge/batch` | `KnowledgeHandler.GetKnowledgeBatch` | `knowledge/read` per ID | 逐条过滤，不暴露无权 ID | `TestWikaScopeFiltersKnowledgeBatch` |
| `GET /api/v1/knowledge/search` | `KnowledgeHandler.SearchKnowledge` | search readable scopes | 不跨 personal space | `TestWikaScopeRejectsLegacyKnowledgeSearchLeak` |
| `GET /api/v1/knowledge/:id/download` | `KnowledgeHandler.DownloadKnowledgeFile` | `knowledge/download` | personal 越权 404 | `TestWikaScopeRejectsDownload` |
| `GET /api/v1/knowledge/:id/preview` | `KnowledgeHandler.PreviewKnowledgeFile` | `knowledge/preview` | personal 越权 404 | `TestWikaScopeRejectsPreview` |
| `POST /api/v1/knowledge/batch-delete` | `KnowledgeHandler.BatchDeleteKnowledge` | `knowledge/write` per ID | 任一无权项拒绝或过滤并审计 | `TestWikaScopeRejectsBatchDeletePersonal` |
| `POST /api/v1/knowledge/batch-reparse` | `KnowledgeHandler.BatchReparseKnowledge` | `knowledge/write` per ID | 任一无权项拒绝或过滤并审计 | `TestWikaScopeRejectsBatchReparsePersonal` |

### 6.4 SuggestionService

`suggest_to_team` 流程：

```text
CreateSuggestion
  -> Load source knowledge
  -> Permission check
  -> AI correct
  -> AI review
  -> Validate structured output
  -> Self-correct on invalid output
  -> Decide approved / needs_confirmation / rejected
  -> ApplyByPolicy or wait for human
```

AI 预审检查：

- 内容质量。
- 团队相关性。
- 敏感信息。
- 重复风险。
- 过期风险。
- AI 修正幅度。
- 是否需要人工确认。

AI 通过条件：

```text
quality_score >= 80
ai_confidence >= 0.85
target_team_fit >= 0.8
duplicate_risk = low
freshness_risk = low
risks is empty
```

自动应用条件：

```text
team.auto_apply_approved = true
ai_decision = approved
deterministic_safety_gate = passed
submitter is member of target team
source knowledge belongs to submitter personal space
source_content_hash unchanged since review
```

否则：

- 有价值但不确定 -> `needs_confirmation`
- 明显不适合 -> `rejected`
- AI 通过但自动应用条件不满足 -> `ai_reviewed`，等待人工或手动应用。

结构化输出必须经过 schema 校验。校验失败时追加更严格提示重试一次；仍失败则进入 `needs_confirmation`。

确定性安全门禁：

- 检测密钥样式、token 样式、邮箱/手机号/身份证等隐私字段、团队配置的敏感词。
- 检测 prompt injection 样式内容，例如要求忽略系统指令、执行命令、泄露凭据。
- 检测 AI 修正幅度；标题或正文改动超过阈值时进入 `needs_confirmation`。
- 检测重复、过期、低置信和目标团队相关性。
- 门禁结果写入 `ai_review`，并记录 `policy_version`。

人工覆盖：

```text
PUT /api/v1/wika/suggestions/:id/human-review
```

人工可以修改：

- 最终决策。
- 标题。
- 正文。
- 标签。
- 目标 KB。
- 审批备注。

应用通过：

- 复制知识到目标团队 KB。
- 重跑 chunk、embedding、索引。
- 写 `knowledge_lineage`。
- 写 audit log。

并发与幂等：

- `CreateSuggestion` 必须接受 `idempotency_key`。
- 同一来源知识到同一目标 KB 只能存在一个未终态 suggestion。
- `Apply` 必须在事务中检查 suggestion 未终态、源内容 hash 未变化、目标 KB 仍可写。
- `Apply` 成功后写 `result_knowledge_id`；重复调用返回已有结果，不重复复制。

### 6.5 EvaluationService

职责：

- 管理 `eval_datasets` 和 `eval_qa_items`。
- 触发评测 run。
- 复用现有 metric 包计算 MRR、Recall@5、NDCG@5。
- 保存 run 级指标和 case 明细。
- 提供趋势查询。
- 支持单条 QA dry-run。

状态：

```text
pending -> running -> completed | failed
```

### 6.6 FreshnessService

职责：

- 根据 `expires_at`、访问聚合、质量分、置信度生成保鲜问题。
- 写入 `freshness_checks` 和 `freshness_check_items`。
- 更新 `wika_knowledge_state.freshness_status`。
- 支持手动处理。

MVP 不做自动外部 URL 重抓。

## 七、MCP 设计

### 7.1 身份

日常 MCP 工具必须使用用户级凭证。

新增 PAT 能力，OAuth 后置：

```text
wika_user_tokens
  id
  user_id
  tenant_id
  name
  token_prefix
  token_hash
  hash_alg
  scopes jsonb
  expires_at
  revoked_at
  created_at
  created_by_ip
  last_used_at
```

约束与索引：

- token 明文只在创建成功响应中展示一次。
- token 格式建议为 `wika_pat_<随机值>`，`token_prefix` 保存前 12-16 位用于展示和排查。
- `token_hash` 使用 SHA-256 或 Argon2id；生产默认 SHA-256 + 服务端 pepper，pepper 走环境变量。
- `(token_hash)` 唯一。
- `(user_id, tenant_id, revoked_at)` 索引用于列表和撤销。
- `(expires_at)` 索引用于清理过期 token。
- `last_used_at` 至多每 5 分钟异步或节流更新一次，避免 MCP 高频调用写放大。

Scope：

- `knowledge:push`
- `knowledge:search`
- `knowledge:read`
- `suggestion:create`

租户级 API key 只保留给管理型工具，不允许用于日常个人知识工具。

鉴权行为：

- 日常 MCP 请求必须带用户级 token。
- token 过期、撤销、scope 不足返回 401/403，不回退到租户 API key。
- token 中的 `tenant_id` 表示默认当前空间，不代表只能访问该空间；可访问范围仍由 `SpaceService.ListUserSpaces` 和 RBAC 解析。
- 管理型 MCP tools 与日常 tools 分组展示；日常 tools 拒绝 `X-API-Key`。

Token 管理 API：

```text
GET    /api/v1/wika/tokens
POST   /api/v1/wika/tokens
DELETE /api/v1/wika/tokens/:id
```

契约：

- `POST` 请求：`{ name, tenant_id?, scopes[], expires_at }`。
- `POST` 响应：`{ id, name, token, token_prefix, scopes, expires_at }`，`token` 只出现一次。
- `GET` 响应不返回 `token_hash`、明文 token、pepper 信息。
- `DELETE` 是撤销，写 `revoked_at`，不是物理删除。
- 默认过期时间最长 90 天；SystemAdmin 不能替用户读取或生成明文 token。
- 创建、撤销、鉴权失败均写 audit log；日志只记录 `token_prefix`。
- API 和 MCP token 校验都要做限流，失败响应不区分 token 不存在、过期或 hash 不匹配。

### 7.2 日常工具

```text
push_knowledge(title, content, source?, tags?, evidence?, expires_at?, idempotency_key?, dry_run?)
search_knowledge(query, limit?, include_team?, format?)
expand_knowledge_result(ids[])
get_my_knowledge(limit?, status?, tag?)
suggest_to_team(knowledge_id, target_space_id?, reason?)
```

`dry_run=true` 时只返回规范化、质量分、疑似重复和建议，不入库。

客户端配置约定：

- 日常工具优先读取 `WEKNORA_PAT`，以 `Authorization: Bearer <token>` 调用 Wika API。
- 兼容期允许 `WEKNORA_API_KEY` 继续服务管理型工具，但日常工具不得把它当作 PAT fallback。
- `WEKNORA_BASE_URL` 仍指向 API v1 根路径，例如 `http://localhost:8080/api/v1`。
- MCP server 启动时如配置了 `WEKNORA_PAT`，必须只注册或启用日常工具的用户级鉴权路径。

stdio 示例：

```json
{
  "mcpServers": {
    "wika": {
      "command": "python",
      "args": ["-m", "weknora_mcp_server"],
      "env": {
        "WEKNORA_BASE_URL": "http://localhost:8080/api/v1",
        "WEKNORA_PAT": "wika_pat_xxx"
      }
    }
  }
}
```

真实验收时必须至少调用一次 `push_knowledge` 和 `search_knowledge`，不能只验证 MCP 进程启动。

### 7.3 管理工具

现有管理型 MCP tools 保留，但与日常工具在权限和文档上分组展示。

## 八、HTTP API 契约

通用规则：

- 所有 Wika API 使用登录用户身份；MCP 日常工具经用户级 token 解析为同样的用户身份。
- 读不到或无权访问个人空间资源时返回 404，不暴露存在性。
- 所有 `kb_id`、`knowledge_id`、`suggestion_id` 在 handler 层进入 service 前必须解析 scope。
- 旧 API 的 KB/knowledge/download/preview/hybrid-search 也必须调用同一套 scope resolver。

P1 权限矩阵：

| 资源/动作 | Personal Owner | Team Viewer | Team Contributor | Team Admin/Owner | SystemAdmin |
|-----------|----------------|-------------|------------------|------------------|-------------|
| 个人知识读写 | 允许本人 | 禁止 | 禁止 | 禁止 | 仅元数据 |
| 团队知识读取 | 成员可读 | 允许 | 允许 | 允许 | 仅按成员身份或元数据 |
| 团队知识写入 | 按团队角色 | 禁止 | 允许 | 允许 | 不绕过团队角色 |
| `suggest_to_team` 创建 | 来源本人允许 | 不适用 | 成员允许提交到所在团队 | 允许 | 不绕过 |
| suggestion 查看 | 提交者、目标团队维护者 | 禁止 | 仅本人提交 | 允许 | 元数据 |
| suggestion 人工审核/应用 | 禁止 | 禁止 | 禁止 | 允许 | 不绕过 |
| eval/freshness 管理 | 不适用 | 只读 | 可 dry-run | 允许配置和运行 | 元数据 |

P1 API 契约：

```text
POST /api/v1/wika/knowledge/push
request: { title?, content, source?, tags?, evidence?, expires_at?, idempotency_key?, dry_run? }
response: { knowledge_id?, normalized, quality_score, duplicate_candidates, status }
权限: 当前用户个人空间 Owner；默认写入个人默认 KB。
幂等: idempotency_key 命中时返回已有 knowledge_id。

POST /api/v1/wika/knowledge/search
request: { query, limit?, include_team?, format? }
response: { results[], truncated, query_plan_id? }
权限: scope resolver 输出 personal/team/shared 可读 KB。
约束: 默认 compact；结果内容标注不可信资料。

POST /api/v1/wika/knowledge/expand
request: { ids[] }
response: { results[] }
权限: 对每个 id 重新做 read scope 校验。

POST /api/v1/wika/suggestions
request: { knowledge_id, target_space_id?, target_kb_id?, reason?, idempotency_key? }
response: { suggestion_id, ai_decision, status, corrected_title, corrected_content, risks, auto_apply_result? }
权限: 来源知识必须属于提交者个人空间；提交者必须属于目标团队。

PUT /api/v1/wika/suggestions/:id/human-review
request: { final_decision, title?, content?, tags?, target_kb_id?, comment? }
response: { suggestion_id, status, final_decision }
权限: 目标团队 Admin/Owner。

POST /api/v1/wika/suggestions/:id/apply
request: {}
response: { suggestion_id, result_knowledge_id, status }
权限: 目标团队 Admin/Owner，或团队策略允许自动应用时由系统执行。
幂等: 已 applied 返回已有 result_knowledge_id。
```

### Space

```text
GET  /api/v1/wika/spaces
GET  /api/v1/wika/spaces/current
POST /api/v1/wika/spaces/team
POST /api/v1/wika/spaces/switch
```

### Intake/Search

```text
POST /api/v1/wika/knowledge/push
POST /api/v1/wika/knowledge/search
POST /api/v1/wika/knowledge/expand
GET  /api/v1/wika/knowledge/mine
```

### Suggestion

```text
POST /api/v1/wika/suggestions
GET  /api/v1/wika/suggestions
GET  /api/v1/wika/suggestions/mine
GET  /api/v1/wika/suggestions/:id
PUT  /api/v1/wika/suggestions/:id/human-review
POST /api/v1/wika/suggestions/:id/apply
```

### Evaluation

```text
GET    /api/v1/wika/kb/:id/eval/datasets
POST   /api/v1/wika/kb/:id/eval/datasets
PUT    /api/v1/wika/kb/:id/eval/datasets/:dataset_id
DELETE /api/v1/wika/kb/:id/eval/datasets/:dataset_id
POST   /api/v1/wika/kb/:id/eval/datasets/:dataset_id/import
GET    /api/v1/wika/kb/:id/eval/datasets/:dataset_id/export
POST   /api/v1/wika/kb/:id/eval/runs
GET    /api/v1/wika/kb/:id/eval/runs
GET    /api/v1/wika/kb/:id/eval/runs/:run_id
POST   /api/v1/wika/kb/:id/eval/dry-run
GET    /api/v1/wika/kb/:id/eval/trend
```

### Freshness

```text
GET  /api/v1/wika/kb/:id/freshness/overview
POST /api/v1/wika/kb/:id/freshness/checks
GET  /api/v1/wika/kb/:id/freshness/checks
GET  /api/v1/wika/kb/:id/freshness/items
PUT  /api/v1/wika/freshness/items/:id
```

### Graph

```text
GET  /api/v1/wika/kb/:id/graph/overview
GET  /api/v1/wika/kb/:id/graph/entities
GET  /api/v1/wika/kb/:id/graph/entities/:entity_id
GET  /api/v1/wika/kb/:id/graph/edges
POST /api/v1/wika/kb/:id/graph/search
```

契约：

- 所有接口先按 KB 做 `read` scope。
- personal KB 只允许 Owner；SystemAdmin 只返回统计，不返回证据文本。
- `graph/search` 返回图谱召回贡献和降级状态，不能绕过主搜索 scope。

### Governance

```text
GET  /api/v1/wika/kb/:id/conflicts
POST /api/v1/wika/kb/:id/conflicts/checks
PUT  /api/v1/wika/conflicts/:id

GET  /api/v1/wika/knowledge/:id/versions
GET  /api/v1/wika/knowledge/:id/versions/:version_id/diff
POST /api/v1/wika/knowledge/:id/versions/:version_id/restore

GET  /api/v1/wika/knowledge/:id/url-refresh
POST /api/v1/wika/knowledge/:id/url-refresh
PUT  /api/v1/wika/url-refresh/:refresh_id/review

GET  /api/v1/wika/kb/:id/eval/schedules
POST /api/v1/wika/kb/:id/eval/schedules
PUT  /api/v1/wika/kb/:id/eval/schedules/:schedule_id
DELETE /api/v1/wika/kb/:id/eval/schedules/:schedule_id

GET  /api/v1/wika/orgs
POST /api/v1/wika/orgs
GET  /api/v1/wika/orgs/:org_id
POST /api/v1/wika/orgs/:org_id/members
DELETE /api/v1/wika/orgs/:org_id/members/:tenant_id
GET  /api/v1/wika/orgs/:org_id/shares
POST /api/v1/wika/orgs/:org_id/shares
DELETE /api/v1/wika/orgs/:org_id/shares/:share_id
```

契约：

- conflict/version/url-refresh 属于 P5，不得在 P1-P3 暗中开启。
- URL refresh 的抓取结果默认进入待确认，不直接覆盖知识正文。
- Organization share 默认 `mode=reference`，撤销后新搜索不能命中；历史 lineage 和 audit metadata 保留。
- P5 不新增 MCP 工具；只回归 `search_knowledge` 对 Organization shared scope 的命中、字段裁剪和撤销后不命中。

P5 权限矩阵：

| 能力/动作 | Team Viewer | Team Contributor | Team Admin/Owner | SystemAdmin | Organization role |
|-----------|-------------|------------------|------------------|-------------|-------------------|
| Conflict list/read | 允许团队内读取 | 允许 | 允许 | 仅元数据 | 无额外权限 |
| Conflict create check | 禁止 | 允许 | 允许 | 禁止绕过团队角色 | 无额外权限 |
| Conflict resolve | 禁止 | 禁止 | 允许 | 禁止绕过团队角色 | 无额外权限 |
| Version list/diff | 按 knowledge read | 按 knowledge read | 按 knowledge read | 仅元数据，个人正文不可读 | 无额外权限 |
| Version restore | 禁止 | 按团队策略可写 | 允许 | 禁止绕过团队角色 | 无额外权限 |
| URL refresh create/review/apply | 禁止 | 创建待确认 job | 允许 review/apply | 禁止绕过团队角色 | 无额外权限 |
| Eval schedule CRUD | 禁止 | 只读或 dry-run | 允许 | 仅元数据 | 无额外权限 |
| Org create/member manage | 禁止 | 禁止 | 源/目标团队 Admin/Owner 才可代表团队 | 禁止绕过团队角色 | org owner/admin 可管理 org 成员 |
| Org share create/revoke | 只能搜索已授权 shared scope | 禁止创建/revoke | 源团队 Admin/Owner 创建；源或 org admin revoke | 仅元数据 | org owner/admin 只能管理授权关系，不能越过团队 Admin/Owner |

P5 错误语义：

| 场景 | 状态码 | 说明 |
|------|--------|------|
| 请求体格式、cron、status、decision 非法 | 400 | 返回字段级错误，不返回内部栈 |
| 未登录或 PAT 无效 | 401 | 不区分 token 不存在、过期或 hash 不匹配 |
| scope 不足且不会泄露个人资源存在性 | 403 | 例如团队 Viewer 尝试创建 eval schedule |
| personal 或 direct-id 资源无权读 | 404 | 不暴露资源存在性 |
| 状态非法转换、重复 active schedule/share、重复 open conflict | 409 | 返回稳定错误码，例如 `STATE_CONFLICT` |
| SSRF、allowed_fields、内容类型等安全策略阻断 | 422 | 返回脱敏策略原因，不返回敏感原文 |
| 限流或 worker 领取冲突 | 429 | API 层限流；worker 内部冲突不暴露给用户 |

P5 状态机：

| 对象 | 状态 | 合法转换 | 非法转换测试 |
|------|------|----------|--------------|
| conflict item | `open -> confirmed/dismissed/resolved`，`confirmed -> resolved` | 终态只允许添加 comment，不再改回 open | resolved 后再次 resolve 返回 409 或幂等终态 |
| knowledge version | 递增 `version_no`，无终态 | restore 生成新版本 | restore 不修改历史 version 内容 |
| url refresh job | `pending -> running -> pending_review -> applied/rejected`，任意抓取失败到 `failed` | `pending_review` 前不可 apply | running 直接 review/apply 返回 409 |
| eval schedule | `enabled=true/false` + `consecutive_failures` | 连续失败达到阈值后 disable 或降频 | disabled 不触发 run |
| org share | `active -> revoked` | revoke 后不可重新 active；新建 share 生成新记录 | revoked share 不进入 ScopeResolver |

P5 API 明细：

```text
GET /api/v1/wika/kb/:id/conflicts?status=open&limit=20&cursor=...
response: { items: [{ id, conflict_type, confidence_score, status, source_knowledge_id, target_knowledge_id, evidence_summary, ai_explanation }], next_cursor? }
权限: 团队成员按 KB read；SystemAdmin 只返回元数据。

POST /api/v1/wika/kb/:id/conflicts/checks
request: { trigger: "manual|knowledge_updated|suggestion_applied", knowledge_ids?: [] }
response: { check_id, status: "pending" }
权限: Team Contributor+；创建后由 worker 执行。
幂等/并发: 同一 KB 同一 trigger 的 running check 返回已有 check 或 409。

PUT /api/v1/wika/conflicts/:id
request: { status: "confirmed|dismissed|resolved", comment? }
response: { id, status, resolved_by?, resolved_at? }
权限: Team Admin/Owner。

GET /api/v1/wika/knowledge/:id/versions
response: { versions: [{ id, version_no, title, content_hash, change_reason, created_by, created_at }] }
权限: knowledge read；SystemAdmin 对 personal 仅元数据。

GET /api/v1/wika/knowledge/:id/versions/:version_id/diff?to_version_id=...
response: { from_version, to_version, title_diff, content_diff, tag_diff, metadata_diff }
权限: knowledge read；无权正文时 diff 字段省略，仅返回元数据。

POST /api/v1/wika/knowledge/:id/versions/:version_id/restore
request: { reason }
response: { restored_from_version_id, new_version_id, knowledge_id, status }
权限: knowledge write；restore 调用现有知识更新和索引链路。

GET /api/v1/wika/knowledge/:id/url-refresh
response: { jobs: [{ id, status, source_url, diff_summary, error_msg?, created_at, reviewed_at? }], schedules: [{ id, enabled, cron_expr, next_run_at, consecutive_failures }] }
权限: knowledge read；SystemAdmin 对 personal 仅元数据。

POST /api/v1/wika/knowledge/:id/url-refresh
request: { source_url?, schedule?: { enabled, cron_expr } }
response: { job_id?, schedule_id?, status }
权限: Team Contributor+ 可创建手动 job；Team Admin/Owner 可创建/更新 schedule。

PUT /api/v1/wika/url-refresh/:refresh_id/review
request: { decision: "apply|reject", comment? }
response: { refresh_id, status, result_version_id?, result_knowledge_id? }
权限: Team Admin/Owner；apply 必须生成新版本。

GET /api/v1/wika/kb/:id/eval/schedules
response: { schedules: [{ id, dataset_id, cron_expr, enabled, next_run_at, last_run_id, consecutive_failures }] }
权限: KB read。

POST /api/v1/wika/kb/:id/eval/schedules
request: { dataset_id, cron_expr, enabled? }
response: { schedule_id, enabled, next_run_at }
权限: Team Admin/Owner。
幂等/并发: 同一 kb_id + dataset_id 只能有一个 enabled schedule。

PUT /api/v1/wika/kb/:id/eval/schedules/:schedule_id
request: { cron_expr?, enabled? }
response: { schedule_id, enabled, next_run_at, consecutive_failures }
权限: Team Admin/Owner。

DELETE /api/v1/wika/kb/:id/eval/schedules/:schedule_id
response: { schedule_id, enabled: false }
权限: Team Admin/Owner；软删除或 disable，不删除 run 历史。

POST /api/v1/wika/orgs
request: { name, member_tenant_ids?: [] }
response: { org_id, name }
权限: 创建者必须是至少一个成员团队 Admin/Owner。

POST /api/v1/wika/orgs/:org_id/members
request: { tenant_id, role: "owner|admin|member" }
response: { org_id, tenant_id, role }
权限: org owner/admin 且该 tenant 的 Team Admin/Owner 确认；personal tenant 禁止加入。

POST /api/v1/wika/orgs/:org_id/shares
request: { source_kb_id, target_tenant_id, mode?: "reference", allowed_fields?: [] }
response: { share_id, status: "active", allowed_fields }
权限: source team Admin/Owner 创建；target team Admin/Owner 或 org admin 确认接收。
约束: allowed_fields 只能是服务端白名单；默认不含 content/chunk/evidence/file。

DELETE /api/v1/wika/orgs/:org_id/shares/:share_id
response: { share_id, status: "revoked", revoked_at }
权限: source team Admin/Owner、target team Admin/Owner 或 org owner/admin。
幂等: 已 revoked 返回 revoked 终态。
```

### Admin Defaults

```text
GET /api/v1/system/admin/kb-defaults
PUT /api/v1/system/admin/kb-defaults
```

实现规则：

- 复用 `system_settings` registry，不新增独立 YAML 配置。
- facade 只聚合 KB 创建需要的默认模型、分块、索引、VLM/ASR、存储引擎字段。
- 只有 SystemAdmin 可写；普通租户 Admin 不能修改系统默认值。

## 九、前端规划

新增页面：

```text
frontend/src/views/wika/
  SpaceList.vue
  PersonalKnowledge.vue
  SuggestionQueue.vue
  EvaluationDashboard.vue
  FreshnessPanel.vue
  GraphExplorer.vue
  KnowledgeVersions.vue
  GovernanceQueue.vue
  AdminKbDefaults.vue
```

改造现有页面：

- 顶部空间切换。
- KB 创建弹窗默认只显示名称、描述、类型。
- 高级配置折叠。
- 知识详情增加“推荐到团队”入口。
- 团队维护者首页突出 `待确认` 队列。
- P4 增加图谱浏览、实体详情和图谱召回贡献说明。
- P5 增加冲突队列、版本 diff/恢复、URL 重抓待确认、定时评测计划、Organization 共享管理。

## 十、安全设计

必须实现：

1. 用户级 MCP token，不复用租户 API key。
2. token scope 校验。
3. 个人空间 API 统一 owner 过滤。
4. SystemAdmin 只读元数据和统计，不读个人正文。
5. `search_knowledge` scope resolver 必须覆盖 personal/team/shared。
6. 搜索结果不得泄露越权 snippet。
7. `suggest_to_team` 创建时校验源知识属于提交者个人空间。
8. 审批详情对无权限用户返回 404。
9. AI 输出必须 schema 校验，不能直接执行非结构化结果。
10. 自动应用必须经过确定性安全门禁。
11. 所有关键动作写 audit log。

字段级数据分类：

| 字段/内容 | 普通成员 | 团队维护者 | SystemAdmin | 说明 |
|-----------|----------|------------|-------------|------|
| space/kb/knowledge ID、标题、状态、时间、质量分 | 按 scope | 按 scope | 可读元数据 | 个人正文不可通过标题外字段泄露 |
| 知识正文、chunk、文件、snippet、证据 | 按 scope | 按 scope | 不可读 | SystemAdmin 不能越权排障读取 |
| AI 修正文、评测 expected_answer、失败 case 文本 | 按 scope | 按 scope | 不可读 | 视为知识正文同级 |
| 图谱实体名称、关系类型 | 按 scope | 按 scope | 可读统计/团队元数据 | 个人图谱只给统计 |
| 图谱证据文本、边证据 chunk | 按 scope | 按 scope | 不可读 | 返回前重新校验 knowledge scope |
| token_prefix | 本人 | 不可读 | 可读脱敏前缀 | 明文和 hash 永不返回 |
| token_hash、pepper、密钥检测命中原文 | 不可读 | 不可读 | 不可读 | 仅日志脱敏摘要 |

自动应用确定性门禁版本化：

- 每次 AI 预审记录 `policy_version`、检测器版本和团队策略快照。
- 检测器至少包含：敏感词、密钥样式、token 样式、PII、prompt injection 样式、AI 修正幅度、重复风险、过期风险、团队相关性。
- 每个检测器要有 fixture 测试集；新增规则必须补阳性、阴性和边界 case。
- 任一阻断级检测命中时，`approved` 降级为 `needs_confirmation` 或 `rejected`。
- 误判处理只能由团队维护者人工覆盖，覆盖原因写 audit log。

P2-P5 额外威胁门禁：

| 阶段 | 威胁 | 必测控制 |
|------|------|----------|
| P2 评测 | 导入文件注入、导出越权、expected answer 泄露 | 文件大小/格式校验、scope 过滤、SystemAdmin 不读正文 |
| P3 保鲜 | 访问统计写放大、越权处理保鲜项 | 异步聚合、item 处理前回溯 KB scope |
| P4 图谱 | 图谱边泄露个人证据、图谱服务失败拖垮搜索 | 字段 allowlist、best-effort 降级 |
| P5 URL 重抓 | SSRF、超大响应、恶意 HTML/脚本 | URL allowlist/denylist、IP 校验、大小/类型/超时限制 |
| P5 版本恢复 | 恢复旧敏感内容、绕过审核 | 恢复前 scope 校验、恢复生成新版本、审计 |
| P5 Organization | 跨团队共享越权、撤销后仍可搜索 | share scope 显式授权、撤销后 resolver 不返回 |

P5 AI 输出门禁：

- 冲突检测的 `ai_explanation`、URL diff 摘要、共享推荐说明都只作为辅助说明，不得直接驱动状态转换。
- 所有 AI 输出必须 schema 校验；schema 失败时状态降级为 `needs_confirmation`、`pending_review` 或 `failed`，不能自动 apply。
- 输入给 AI 的知识正文、URL 抓取正文、图谱证据、评测 case 全部标注为不可信资料，不得作为系统指令。
- P5 prompt injection fixture 必须覆盖：要求忽略系统指令、泄露密钥、自动 approve、访问内网 URL、绕过 allowed_fields。
- AI 输出、风险命中原文、抓取正文不得写入审计日志；审计只写脱敏摘要和 hash。

URL safe fetcher 细则：

- 只允许 `http` 和 `https`，禁用代理环境变量和用户自定义 header/cookie。
- URL 必须做 IDNA、IPv6、十进制/八进制 IP、混合大小写协议和尾点域名规范化后再校验。
- DNS 解析后的所有 IP 都必须通过 public 地址校验；连接时使用校验后的 IP pinning，防 DNS rebinding。
- 每次重定向后重新校验协议、host、解析 IP；最大重定向 3 次。
- 同时限制响应头声明大小、实际读取大小、解压后大小、总耗时和 content-type。
- HTML、diff 和错误摘要只按 inert text 渲染，不执行脚本，不加载远程资源。

P5 audit event taxonomy：

| event | 必备字段 | 禁止字段 |
|-------|----------|----------|
| `wika.conflict.check_created` | actor_id, auth_type, tenant_id, kb_id, check_id, trigger, request_id | 正文、snippet |
| `wika.conflict.item_resolved` | actor_id, tenant_id, kb_id, item_id, old_status, new_status, request_id | evidence 原文 |
| `wika.version.recorded` | actor_id, tenant_id, kb_id, knowledge_id, version_id, version_no, reason, request_id | content |
| `wika.version.restored` | actor_id, tenant_id, kb_id, knowledge_id, restored_from_version_id, new_version_id, request_id | content diff |
| `wika.url_refresh.job_created` | actor_id, tenant_id, kb_id, knowledge_id, job_id, source_url_hash, request_id | source_url 明文 |
| `wika.url_refresh.job_failed` | actor_id/system, tenant_id, kb_id, job_id, failure_code, request_id | 抓取正文、内网地址明文 |
| `wika.url_refresh.reviewed` | actor_id, tenant_id, kb_id, job_id, decision, old_status, new_status, result_version_id, request_id | fetched_content |
| `wika.eval_schedule.updated` | actor_id, tenant_id, kb_id, schedule_id, enabled, cron_expr_hash, request_id | QA 正文 |
| `wika.eval_schedule.run_failed` | actor_id/system, tenant_id, kb_id, schedule_id, run_id, failure_code, consecutive_failures | expected_answer |
| `wika.org_share.created` | actor_id, org_id, source_tenant_id, source_kb_id, target_tenant_id, share_id, allowed_fields, request_id | 正文、证据 |
| `wika.org_share.revoked` | actor_id, org_id, share_id, source_tenant_id, target_tenant_id, request_id | 正文、证据 |

旧接口必须接入 personal scope：

| 区域 | 典型接口 | 要求 |
|------|----------|------|
| KB 列表/详情 | `/knowledge-bases`、`/knowledge-bases/:id` | 不列出无权访问的个人 KB |
| KB hybrid search | `/knowledge-bases/:id/hybrid-search` | 对 personal KB 做 Owner 校验 |
| 知识详情 | `/knowledge/:id`、`/knowledge/batch` | batch 内逐条过滤，越权项不暴露存在性 |
| 知识搜索 | `/knowledge/search`、`/knowledge-search` | 不跨 personal space 搜索 |
| 文件读取 | `/knowledge/:id/download`、`/knowledge/:id/preview` | 下载/预览必须按 knowledge 回溯 KB scope |
| 评测/保鲜/图谱 | 新旧相关接口 | 统一通过 scope resolver |

安全测试必须覆盖：

- A/B 个人知识 IDOR。
- 团队 Viewer 越权审批。
- 租户 API key 不能调用日常个人知识工具。
- SystemAdmin 不能读取个人正文。
- 恶意知识中的 prompt injection 不触发自动团队发布。
- 旧接口不能读取、搜索、下载或预览他人个人知识。

## 十一、分期实施

### 通用 TDD 工作规则

每个 PR 按 RED -> GREEN -> REFACTOR 执行：

1. RED：先写一个能表达阶段门禁的失败测试。失败原因必须是“能力不存在或规则未满足”，不是测试环境坏。
2. GREEN：只写通过当前测试所需的最小实现。
3. REFACTOR：补齐错误处理、日志、指标和重复代码收敛，再跑阶段回归。

推荐测试落点：

| 层级 | 测试文件位置 | 覆盖重点 |
|------|--------------|----------|
| 类型和 migration | `internal/types/*_test.go`、`internal/types/*migration*_test.go` | GORM 映射、DDL 约束、up/down 可回滚 |
| Wika service | `internal/wika/<module>/*_test.go` | 事务、不变量、幂等、策略决策 |
| Handler/API | `internal/handler/wika_*_test.go`、`internal/router/*_test.go` | 请求/响应、状态码、scope、错误语义 |
| Middleware/Auth | `internal/middleware/*_test.go` | PAT、JWT、租户 API key 分组、scope 拒绝 |
| MCP | `mcp-server/tests/*` 或现有 Python 测试入口 | 工具 schema、PAT header、真实 HTTP 调用 |
| 前端 | `frontend/src/views/wika/**/__tests__` 或项目现有测试约定 | 主路径表单、队列筛选、权限态 |

阶段内如同时改旧 WeKnora API，必须把旧接口越权测试放在同一个 PR，不能留到后续补。

### P0 方案校准

交付：

- 本文档和需求文档确认。
- ADR：
  - Space 复用 Tenant。
  - 一人一个个人空间由 `user_personal_spaces` 承载。
  - 不使用 KB visibility 表达个人知识。
  - 日常 MCP 使用用户级 token。
  - `search_knowledge` 使用 scope-aware hybrid search。
  - `suggest_to_team` 使用 AI 预审三态决策。
  - 团队自动应用默认关闭，开启后仍需确定性安全门禁。
  - 旧 API 必须接入 personal scope。

门禁：

- ADR-01 到 ADR-10 全部确认，任何反向实现必须先改 ADR。
- migration 编号、旧 API inventory、ScopeResolver 契约、P1a 测试矩阵已确认。
- 不能在 P0 后直接进入 `push_knowledge` 或前端页面开发。

### P1a 安全与空间底座

目标：先建立不可绕过的空间、默认 KB、用户 token 和旧 API scope。

交付：

- `tenants.space_type` / personal space。
- `user_personal_spaces`。
- 默认 KB。
- 空间策略 `wika_space_policies`，团队自动应用默认关闭。
- 用户级 MCP token。
- ScopeResolver。
- 旧 KB/knowledge/search/download/preview API personal scope 改造。

验证：

- A/B 个人空间互相 404 或空结果。
- 租户 API key 不能调用日常 MCP tools。
- SystemAdmin 只能读取个人空间元数据。
- 旧 API 的列表、详情、batch、search、download、preview、hybrid-search 全部通过越权测试。

建议 PR 序列：

1. migration：space、personal mapping、defaults、policies、tokens。
2. SpaceService：注册/首次访问创建个人空间、默认 KB、空间列表。
3. TokenService：创建、列表、撤销、scope 校验、MCP 鉴权分组。
4. ScopeResolver：KB/knowledge/file/search 资源解析。
5. 旧 API scope 接入和 A/B 安全测试。

### P1b 生产与检索闭环

目标：让用户通过 Web/MCP 写入个人知识，并能从个人+团队 scope 检索。

交付：

- `wika_knowledge_state` 最小字段。
- `knowledge_access_daily`。
- `push_knowledge`。
- `search_knowledge` compact。
- `expand_knowledge_result`。
- `get_my_knowledge`。

验证：

- Web 和 MCP 都能写入。
- 个人 + 团队检索可用。
- compact 默认不返回过长正文，expand 重新做 scope。
- 搜索命中写入访问聚合；聚合失败不影响响应。
- 幂等键重复调用不重复创建知识。

建议 PR 序列：

1. migration：knowledge state、access daily。
2. IntakeService：Normalize、Score、DuplicateCheck、Persist、Index。
3. SearchService：scope fan-out、RRF merge、quality/freshness rerank、compact/expand。
4. MCP 日常 tools：`push_knowledge`、`search_knowledge`、`expand_knowledge_result`、`get_my_knowledge`。
5. 前端：个人知识列表、手动创建、空间切换最小入口。

### P1c 团队推荐与 AI 预审

目标：完成个人到团队的治理流转。

交付：

- `knowledge_suggestions`。
- `knowledge_lineage`。
- `suggest_to_team` AI 修正、质量审核、风险审核、团队适配判断。
- 人工覆盖和应用。
- 团队复制入库。
- 团队自动应用策略入口。

验证：

- `通过 / 待确认 / 不通过` 三态可构造。
- 默认不开启自动应用时，AI 通过不会自动进入团队。
- 开启自动应用且安全门禁通过时，AI 通过可自动复制到团队。
- 恶意知识、敏感内容、旧接口越权访问均被阻断。
- 重复调用 `suggest_to_team` 不重复复制团队知识。
- AI 结构化输出 schema 校验失败会重试一次，仍失败进入 `needs_confirmation`。

建议实施顺序：

1. migration：suggestion、lineage、部分唯一索引。
2. SuggestionService：创建、AI review、schema 校验、三态决策。
3. DeterministicSafetyGate：检测器、策略版本、fixture。
4. Apply：事务锁、source hash 校验、复制知识、lineage、audit。
5. 前端：推荐队列、人工覆盖、团队策略。

### P2 质量闭环

交付：

- eval dataset。
- eval QA items。
- eval runs。
- eval run items。
- 趋势接口。
- 单条 dry-run。
- CSV/JSON 导入导出。
- 替换现有 `internal/application/service/evaluation.go` 中仅内存保存任务的语义：Wika 评测 run/case 必须以 DB 为 source of truth。

验证：

- 5 条 QA 可评测。
- 指标和 case 明细可复查。
- 缺少 `expected_knowledge_ids` 和 `expected_chunk_ids` 的 QA 只能 dry-run。
- 导入非法格式、超大文件、越权导出均被拒绝。
- run 记录检索参数、模型版本、索引策略和 dataset_version。

建议 PR 序列：

1. migration：eval dataset、qa items、runs、run items。
2. EvaluationService：CRUD、import/export、dry-run、run worker。
3. metric adapter：复用现有 metric 包，统一 MRR/Recall@5/NDCG@5 口径。
4. API 和前端趋势面板。
5. 权限、安全、导入导出测试。

### P3 保鲜闭环

交付：

- freshness checks/items。
- 保鲜面板。
- 手动处理动作。
- 访问聚合驱动的长期未访问扫描。
- freshness scanner worker。
- 访问聚合 flush worker。

验证：

- 过期、将过期、长期未访问、低质量均可构造和处理。
- 检索热路径不直接更新 `knowledges` 主表。
- scanner 失败不影响搜索和知识读取。
- 处理动作写操作者、处理前后状态和 audit log。

建议 PR 序列：

1. migration：freshness checks/items。
2. AccessFlushWorker：批量 upsert `knowledge_access_daily`。
3. FreshnessScanner：按阈值生成 items，更新 `wika_knowledge_state`。
4. API：overview、checks、items、处理动作。
5. 前端：保鲜面板和处理流。

### P4 发现增强

交付：

- 图谱浏览。
- 图谱详情。
- 图谱增强检索权重调优。
- 图谱读模型和同步任务。

验证：

- 团队图谱实体、关系、证据来源可分页查看。
- 个人图谱只允许 Owner；SystemAdmin 不返回个人证据文本。
- 图谱检索开启后能看到召回贡献；关闭后主搜索不受影响。
- 图谱服务失败时搜索返回降级状态。

建议 PR 序列：

1. migration：graph entities/edges 读模型。
2. GraphReadService：同步抽取结果、分页、详情、证据 scope。
3. SearchService：图谱召回权重配置和降级指标。
4. 前端：图谱浏览和实体详情。

### P5 高级治理

交付：

- 冲突检测。
- 版本 diff 和恢复。
- 自动 URL 重抓。
- 定时评测。
- Organization 跨团队引用共享。

验证：

- 冲突候选可生成、确认、驳回、解决，AI 不自动覆盖知识。
- 每次知识变更形成版本；恢复旧版本生成新版本。
- URL 重抓 SSRF fixture 全部通过，抓取结果默认进入待确认。
- 定时评测可启停、失败降频、保留 run/case 明细。
- Organization share 授权、引用检索、撤销后不再命中。

建议 PR 序列：

1. governance migration：conflicts、versions、url refresh、eval schedules、organization shares。
2. VersionService：写入 hook、diff、restore。
3. ConflictService：候选生成、AI 解释、人工状态流转。
4. URLRefreshWorker：SSRF 防护、抓取、diff、人工应用。
5. EvalScheduler：cron、失败降频、run 复用。
6. OrganizationShareService：授权、ScopeResolver shared scope、撤销。

P5 Implementation Map：

| 子阶段 | 稳定锚点 | 表 | Service | API | 前端 | feature flag | 完成证据 |
|--------|----------|----|---------|-----|------|--------------|----------|
| P5a Conflict | `WIKA-P5-CONFLICT` | `wika_conflict_checks/items` | `governance/conflict` | `/wika/kb/:id/conflicts*`、`/wika/conflicts/:id` | 冲突队列、详情抽屉 | `wika.governance.conflict.enabled` | 候选生成、状态流转、AI 不改正文 |
| P5b Version | `WIKA-P5-VERSION` | `wika_knowledge_versions` | `governance/version` | `/wika/knowledge/:id/versions*` | diff/restore | `wika.governance.version.enabled` | 关键写路径记录版本，restore 新版本 |
| P5c URL Refresh | `WIKA-P5-URL-REFRESH` | `wika_url_refresh_jobs/schedules` | `governance/urlrefresh` | `/wika/knowledge/:id/url-refresh*`、`/wika/url-refresh/:id/review` | 待确认更新队列 | `wika.governance.url_refresh.enabled` | SSRF fixture、pending review、apply 生成版本 |
| P5d Eval Schedule | `WIKA-P5-EVAL-SCHEDULE` | `wika_eval_schedules` | `governance/evalschedule` | `/wika/kb/:id/eval/schedules*` | 评测计划 | `wika.governance.eval_schedule.enabled` | DB lease、失败降频、run 明细 |
| P5e Org Share | `WIKA-P5-ORG-SHARE` | `wika_organizations/org_members/org_shares` | `governance/orgshare`、`scope` | `/wika/orgs*`、`/wika/orgs/:id/shares*` | Organization 和共享管理 | `wika.governance.org_share.enabled` | allowed_fields 裁剪、revoke 后不命中 |

P5 命名词汇表：

| 术语 | 含义 |
|------|------|
| check | 一次治理扫描任务，例如一次冲突检测 |
| item | check 产生的可处理候选项 |
| reference | 共享检索引用，默认不复制正文 |
| snapshot | 未来可选的只读快照共享；P5 默认不启用 |
| Organization | 跨团队共享容器，不替代 team space |
| org member | 加入 Organization 的团队空间及其 org role |
| share | 从源 KB 授权到目标团队的引用共享记录 |
| shared scope | ScopeResolver 根据 active share 输出的可搜索范围 |
| allowed_fields | 共享命中可返回字段的服务端白名单 |

### P0-P5 任务卡索引

下表用于把方案直接拆成实现任务。每张任务卡完成前必须先写对应 RED 测试。

| 任务卡 | 首个 RED 测试 | 主要实现落点 | 阶段验收命令或证据 |
|--------|---------------|--------------|--------------------|
| P1a-1 Space migration | `tenants.space_type` check 约束、personal mapping 唯一性失败 | `migrations/versioned/000090*`、`internal/types/tenant.go`、`internal/types/wika_space.go` | migration up/down、`go test ./internal/types` |
| P1a-2 SpaceService | 用户首次访问后存在 personal tenant、Owner 成员、默认 KB | `internal/wika/space` | service 单测覆盖重复调用幂等 |
| P1a-3 User Token | token 明文只返回一次，GET 不返回 hash，租户 API key 调日常工具被拒绝 | `internal/wika/auth`、`internal/handler/wika_token.go`、`internal/middleware/auth.go` | handler/middleware 测试 |
| P1a-4 ScopeResolver | A 不能读 B personal KB/knowledge/file/search | `internal/wika/scope`、旧 handler/service 接入点 | 旧 API A/B 越权测试 |
| P1b-1 Intake | 空 content 400；幂等键重复不重复创建；dry_run 不入库 | `internal/wika/intake`、`internal/handler/wika_knowledge.go` | Web/API push 冒烟 |
| P1b-2 Search | compact 不返回全文；expand 重新校验 scope；无权 ID 被过滤 | `internal/wika/search`、现有 hybrid search adapter | personal+team 搜索证据、access upsert 证据 |
| P1b-3 MCP 日常工具 | `WEKNORA_PAT` 走 Bearer，`WEKNORA_API_KEY` 不可调用日常工具 | `mcp-server/weknora_mcp_server.py`、MCP tests | stdio 或 HTTP 真实调用截图/日志 |
| P1c-1 Suggestion 创建 | 三态 fixture 可构造；AI schema 错误重试后待确认 | `internal/wika/suggestion`、`internal/handler/wika_suggestion.go` | suggestion API 测试 |
| P1c-2 SafetyGate/Apply | 默认不自动应用；开启后安全门禁失败降级；重复 apply 不复制 | `internal/wika/suggestion/safety_*`、`knowledge_lineage` store | AI fixture、事务幂等、安全测试 |
| P2-1 Dataset/QA | 无 expected IDs 不能进入正式指标 | `internal/wika/evaluation`、eval migrations | QA CRUD/import/export 测试 |
| P2-2 Eval run | run 记录 search_config、dataset_version、case 明细 | `internal/wika/evaluation`、metric adapter | 5 条 QA 正式 run + dry-run 证据 |
| P3-1 Access flush | 搜索热路径不更新 `knowledges`，flush 失败不影响搜索 | `internal/wika/freshness` 或 `internal/wika/search/access` | DB 写入 spy 或 repository 测试 |
| P3-2 Freshness scanner | 过期、将过期、长期未访问、低质、低置信均生成 item | `internal/wika/freshness`、worker 注册 | scanner fixture 和处理审计 |
| P4-1 Graph read model | SystemAdmin 不返回个人证据文本 | `internal/wika/graph`、graph migrations | 分页、详情、字段 allowlist 测试 |
| P4-2 Graph search | 图谱失败时主搜索成功并返回降级标识 | `internal/wika/search`、`internal/wika/graph` | best-effort 降级测试 |
| P5a-1 Conflict migration | open conflict 唯一约束失败 | `migrations/versioned/000097*`、`internal/types/wika_governance*.go` | migration up/down、`go test ./internal/types` |
| P5a-2 Conflict check worker | 同一 running check 不重复领取 | `internal/wika/governance/conflict` | DB lease、候选生成 fixture |
| P5a-3 Conflict API | resolved 后再次修改返回 409 或幂等终态 | `internal/handler/wika_governance_conflict.go` | API 状态流转、审计事件 |
| P5b-1 Version migration | `(knowledge_id, version_no)` unique | `migrations/versioned/000098*`、`internal/types/wika_version.go` | migration up/down |
| P5b-2 Version hooks | 手动更新、旧 API 更新、suggestion apply、URL apply、restore 均记录版本 | `internal/wika/governance/version`、知识更新链路适配点 | 写路径 fixture |
| P5b-3 Version API | restore 生成新版本，不覆盖历史 | `internal/handler/wika_governance_version.go` | diff/restore/scope 测试 |
| P5c-0 URL refresh migration | job 状态 check、schedule 启用唯一、lease 字段存在 | `migrations/versioned/000099*`、`internal/types/wika_url_refresh.go` | migration up/down |
| P5c-1 Safe fetcher | 内网、metadata、重定向、DNS rebinding、超大响应均阻断 | `internal/wika/governance/urlrefresh/safefetch` | SSRF fixture |
| P5c-2 URL refresh job | 正常 URL 进入 pending_review，不改知识 | `internal/wika/governance/urlrefresh` | job 状态机、worker lease |
| P5c-3 URL review/apply | apply 生成新版本；reject 不改知识 | `internal/handler/wika_governance_urlrefresh.go`、`VersionService` | API 冒烟、审计 |
| P5c-4 URL schedule | 连续失败后 disable 或延后 next_run_at | `wika_url_refresh_schedules`、worker 注册 | cron/失败计数测试 |
| P5d-1 Eval schedule migration | 同一 `kb_id + dataset_id` 只能一个 enabled schedule | `migrations/versioned/000100*` | migration up/down |
| P5d-2 Eval scheduler worker | 多实例不会重复创建 run | `internal/wika/governance/evalschedule` | DB lock、失败降频 |
| P5d-3 Eval schedule API | cron 非法 400，disabled 不触发 | `internal/handler/wika_eval_schedule.go` | handler/API 测试 |
| P5e-1 Org model migration | org/member/share 约束和 revoke 状态 | `migrations/versioned/000101*`、`internal/types/wika_org_share.go` | migration up/down |
| P5e-2 Org/member API | personal tenant 禁止加入 Organization | `internal/handler/wika_org.go` | 角色和接收方确认测试 |
| P5e-3 Share authorization | allowed_fields 只能服务端白名单 | `internal/wika/governance/orgshare` | share/create/revoke 测试 |
| P5e-4 Shared scope search | revoke 后 ScopeResolver 不再返回 shared scope | `internal/wika/scope`、`internal/wika/search` | `search_knowledge` shared scope 回归 |

### P5 子能力实施契约

P5 只能在 P1-P4 门禁通过后开启。所有 P5 API 默认放在功能开关后，或按阶段路由注册，不得在数据模型未完成时暴露空实现。

#### ConflictService

职责：

- 生成冲突检测任务。
- 将相似、重复、过期覆盖、范围重叠等候选写入 `wika_conflict_items`。
- 提供人工确认、驳回、解决状态流转。

最小方法：

```text
CreateCheck(ctx, actor, kbID, trigger) -> checkID
ListItems(ctx, actor, kbID, filters) -> []ConflictItem
ResolveItem(ctx, actor, itemID, status, comment) -> ConflictItem
```

实现规则：

- 候选生成可复用 SearchService 和 source hash，不在 P5 自建检索引擎。
- `evidence` 默认只存定位、hash 和摘要；返回正文前必须重新做 knowledge read scope。
- AI 解释只作为辅助字段，不能作为自动覆盖或删除的依据。

必测：

- 无权 KB 不能创建 check。
- 同一未终态冲突不会重复生成。
- 确认/驳回/解决只改 conflict item，不改原知识正文。

#### VersionService

职责：

- 在知识标题、正文、标签、状态等关键字段变更前后记录版本。
- 提供相邻版本 diff。
- 恢复旧版本。

最小方法：

```text
RecordVersion(ctx, knowledgeID, snapshot, reason, actorID)
ListVersions(ctx, actor, knowledgeID)
Diff(ctx, actor, knowledgeID, fromVersion, toVersion)
Restore(ctx, actor, knowledgeID, versionID, reason) -> newVersionID
```

实现规则：

- `version_no` 在事务中按 knowledge 加锁递增。
- restore 调用现有知识更新链路和索引链路，然后生成新版本。
- SystemAdmin 对个人知识版本仍只能读元数据。

必须版本化的写路径：

- Web 手动更新知识标题、正文、标签、状态。
- 旧 WeKnora 知识更新 API。
- `suggest_to_team` apply 复制或更新团队知识。
- URL refresh review decision 为 `apply`。
- freshness 处理动作导致知识正文、状态或有效期变化。
- restore 本身。

必测：

- 连续更新形成递增版本。
- restore 不覆盖历史版本。
- A 不能读取 B personal knowledge 版本正文。

#### URLRefreshService

职责：

- 对来源 URL 进行安全重抓。
- 生成新旧内容 diff 和待确认更新。
- 人工确认后写入知识并形成新版本。

最小方法：

```text
CreateRefreshJob(ctx, actor, knowledgeID)
CreateOrUpdateSchedule(ctx, actor, knowledgeID, cronExpr, enabled)
RunRefreshJob(ctx, jobID)
ReviewRefresh(ctx, actor, jobID, decision, comment)
RunDueSchedules(ctx, now)
```

SSRF 防护必须包含：

- 仅允许 `http` 和 `https`。
- 禁用代理环境变量、用户 cookie 和用户自定义 header。
- URL 做 IDNA、IPv6、十进制/八进制 IP、尾点域名、大小写协议规范化。
- DNS 解析后的所有 IP 都不能是 private、loopback、link-local、multicast、reserved、云元数据地址。
- 连接时使用校验后的 IP pinning，防 DNS rebinding。
- 每次重定向后重新校验 URL、协议、host 和解析 IP。
- 限制响应头声明大小、实际读取大小、解压后大小、总耗时、content-type。
- 抓取失败不改变原知识。
- HTML 和 diff 只按 inert text 渲染，不执行脚本或加载远程资源。

实现规则：

- 手动 job 和 schedule job 使用同一 `wika_url_refresh_jobs` 状态机。
- schedule 只生成 job；job runner 负责抓取；review apply 负责调用知识更新链路和 VersionService。
- worker 使用 DB lease 领取 job；同一 job 重复领取必须被拒绝。

必测 fixture：

- `http://127.0.0.1`、`http://[::1]`、`http://169.254.169.254`、内网域名、重定向到内网、DNS rebinding、超大响应、解压炸弹、非 HTML/文本类型全部被拒绝。
- 正常 URL 成功后状态为 `pending_review`，不会直接覆盖知识正文。

#### EvalScheduleService

职责：

- 为 P2 评测 run 增加定时触发。
- 记录最近运行、下一次运行、连续失败次数。
- 失败达到阈值后停用或降频。

最小方法：

```text
CreateSchedule(ctx, actor, kbID, datasetID, cronExpr)
UpdateSchedule(ctx, actor, scheduleID, payload)
RunDueSchedules(ctx, now)
```

实现规则：

- 同一 `kb_id + dataset_id` 只能有一个启用计划。
- worker 必须使用 DB 锁或等价机制避免多实例重复触发。
- schedule 只创建 eval run，不复制评测逻辑。

必测：

- cron 解析错误返回 400。
- 连续失败达到阈值后状态变化和 audit log 存在。
- 禁用 schedule 不会触发新 run。

#### OrganizationShareService

职责：

- 管理 Organization、成员团队和跨团队 KB 引用共享。
- 将 active share 暴露给 ScopeResolver。
- 撤销后阻止新搜索命中。

最小方法：

```text
CreateOrganization(ctx, actor, payload)
AddOrgMember(ctx, actor, orgID, tenantID, role)
CreateShare(ctx, actor, orgID, sourceKBID, targetTenantID, allowedFields)
RevokeShare(ctx, actor, shareID)
ListSharedScopes(ctx, actor, tenantID)
```

实现规则：

- P5 默认 `mode=reference`，不复制正文。
- `allowed_fields` 默认只允许 ID、标题、来源团队、质量分、保鲜状态，不含正文、chunk、证据、文件。
- SearchService 命中 shared scope 后仍要用 ScopeResolver 输出的 `allowed_fields` 裁剪结果。
- revoke 后 ScopeResolver 不能再返回该 share；历史访问日志和 lineage metadata 保留。
- personal tenant 禁止加入 Organization，personal KB 禁止作为 Organization share source。
- `allowed_fields` 只能从服务端白名单枚举中选择，客户端传入未知字段返回 400。
- `CreateShare` 要求 source team Admin/Owner；target team Admin/Owner 或 org owner/admin 必须确认接收。
- `RevokeShare` 允许 source team Admin/Owner、target team Admin/Owner 或 org owner/admin 执行。

必测：

- 未加入接收团队的用户不能通过 org share 搜索。
- revoke 后同一 query 不再返回共享结果。
- SystemAdmin 不因 Organization 共享获得个人或团队正文读取权。

## 十二、迁移计划

建议迁移编号：

| 编号 | 内容 |
|------|------|
| 000090 | Wika space 类型和 `user_personal_spaces` |
| 000091 | Wika 默认 KB、空间策略、用户级 MCP token |
| 000092 | Wika knowledge state、access daily |
| 000093 | knowledge suggestions、lineage、suggestion 幂等约束 |
| 000094 | eval datasets / qa / runs / run items |
| 000095 | freshness checks / items |
| 000096 | graph entities / graph edges 读模型 |
| 000097 | conflict checks / conflict items |
| 000098 | knowledge versions |
| 000099 | url refresh jobs / url refresh schedules |
| 000100 | eval schedules |
| 000101 | organizations / org members / org shares |

当前上游迁移已到 `000063`，`000090+` 仍留有缓冲。

P5 migration 必测约束：

- `wika_conflict_items` 同一 `source_knowledge_id + target_knowledge_id + conflict_type` 未终态唯一。
- `wika_knowledge_versions(knowledge_id, version_no)` 唯一。
- `wika_url_refresh_schedules(knowledge_id, source_url)` 启用状态唯一。
- `wika_eval_schedules(kb_id, dataset_id)` 启用状态唯一。
- `wika_org_members(org_id, tenant_id)` 唯一。
- `wika_org_shares` active 状态下同一 `source_kb_id + target_tenant_id` 唯一。
- down migration 必须按 org share/member/org、eval schedule、url refresh schedule/job、version、conflict 的依赖顺序回滚。

## 十三、冲突风险

| 文件/区域 | 改动 | 风险 | 说明 |
|-----------|------|------|------|
| `migrations/versioned/000090+` | 新增迁移 | 低 | 编号避开上游 |
| `internal/types/tenant.go` | 新增字段 | 中 | Space=Tenant 必须改 |
| `internal/types/interfaces/**` | 追加接口方法 | 高 | Go 接口追加会破坏所有实现，优先在 `internal/wika/**` 使用组合 service |
| `internal/router/router.go` | 新增 Wika 路由 | 低 | 末尾追加 |
| `internal/container/container.go` | 注册 Wika service | 中 | DI 组装点 |
| `internal/wika/**` | 新增 | 低 | 主体实现位置 |
| `mcp-server/**` | 新增日常 tools 和用户 token 配置 | 中 | 现有文件较大，长期应拆模块 |
| `frontend/src/router/index.ts` | 新增路由 | 低 | 末尾追加 |
| `frontend/src/views/wika/**` | 新增页面 | 低 | 独立页面 |
| 现有 KB/knowledge/search/download/preview API | 接入 personal scope | 高 | 必须防旧接口越权 |
| 现有知识更新链路 | P5b Version hook | 高 | 必须覆盖 Web、旧 API、suggestion apply、URL apply、restore |
| `internal/wika/search` | P4 图谱贡献、P5e shared scope 字段裁剪 | 高 | SearchService 必须按 ScopeResolver allowed_fields 输出 |
| `internal/wika/scope` | P5 direct-id 和 Organization shared scope | 高 | direct-id 必须回溯父 KB/knowledge；revoke 后不可命中 |
| P5 worker 注册 | conflict/url-refresh/eval schedule worker | 中 | 必须有 feature flag、DB lease 和停用路径 |
| URL fetcher 网络边界 | safe fetcher | 高 | 禁代理、IP pinning、重定向重校验和大小限制必须有 fixture |
| `internal/router/router.go` / DI | Governance API 注册 | 中 | P5 API 按子阶段注册，不能暴露空实现 |
| `frontend/src/views/wika/**` | P5 队列、diff、schedule、org share 页面 | 中 | 前端只消费 API 状态机，不在前端拼权限 |

## 十四、完成门禁

不能只靠代码存在判断完成。每期必须提供证据：

- 数据库迁移 up/down。
- Go 单元测试。
- API 冒烟。
- 涉及 MCP 的阶段必须提供 stdio 或 HTTP 真实调用；P5 不新增 MCP 工具，只回归 `search_knowledge` shared scope。
- A/B 越权安全测试。
- 前端主路径 E2E。
- 对应指标或状态可查。

不满足以下条件时不得声明完成：

- 只能 push 但搜不到。
- 只隐藏 UI 但 API 可越权。
- 只注册 MCP tool 但没有真实调用。
- 只存 run 但没有 case 明细。
- 只加过期字段但没有扫描和处理状态。
- AI 预审没有 schema 校验和人工覆盖。
- 团队未开启自动应用时，AI 通过结果仍被自动发布。
- 旧 API 仍可访问他人个人空间内容。
- `suggest_to_team` 重复调用会重复复制团队知识。
- 评测只有 run 汇总，没有 case 明细和失败原因。
- 保鲜只有状态字段，没有扫描任务、处理动作和访问聚合来源。
- 图谱接口绕过 KB scope 或图谱失败导致主搜索失败。
- URL 重抓没有 SSRF fixture 和人工确认流程。
- 版本恢复覆盖历史版本，而不是生成新版本。
- Organization 撤销共享后，新搜索仍能命中共享内容。

阶段完成证据：

| 阶段 | 必须提供的证据 |
|------|----------------|
| P1a | migration up/down、A/B 旧 API 越权测试、token 创建/撤销/拒绝租户 API key |
| P1b | Web/MCP push、search compact/expand、幂等、access flush 或批量 upsert 证据 |
| P1c | AI 三态 fixture、自动应用关闭/开启两组测试、人工覆盖、重复 apply 幂等 |
| P2 | 导入导出、dry-run、正式 run、趋势、case 明细、越权导出阻断 |
| P3 | scanner 构造 5 类问题、处理动作审计、检索热路径无主表写入 |
| P4 | 图谱读模型分页、实体详情、图谱检索降级、SystemAdmin 字段 allowlist |
| P5 | 冲突处理、版本恢复、SSRF fixture、定时评测失败降频、Organization 撤销后 resolver 不返回 |

P5 每卡证据模板：

| 任务卡 | 测试命令 | fixture / API 冒烟 | 审计证据 |
|--------|----------|--------------------|----------|
| P5a-1/P5a-2/P5a-3 Conflict | `go test ./internal/wika/governance/conflict ./internal/handler` | `POST /wika/kb/:id/conflicts/checks`、`PUT /wika/conflicts/:id` | `wika.conflict.check_created`、`wika.conflict.item_resolved` |
| P5b-1/P5b-2/P5b-3 Version | `go test ./internal/wika/governance/version ./internal/handler` | `GET /wika/knowledge/:id/versions`、`POST /restore` | `wika.version.recorded`、`wika.version.restored` |
| P5c-0/P5c-1/P5c-2/P5c-3/P5c-4 URL refresh | `go test ./internal/types ./internal/wika/governance/urlrefresh ./internal/handler` | migration up/down、SSRF fixture；`POST /url-refresh`、`PUT /url-refresh/:id/review` | `wika.url_refresh.job_created`、`job_failed`、`reviewed` |
| P5d-1/P5d-2/P5d-3 Eval schedule | `go test ./internal/wika/governance/evalschedule ./internal/handler` | cron fixture；`POST /eval/schedules`、disable 后 worker 不触发 | `wika.eval_schedule.updated`、`run_failed` |
| P5e-1/P5e-2/P5e-3/P5e-4 Org share | `go test ./internal/wika/governance/orgshare ./internal/wika/scope ./internal/wika/search ./internal/handler` | share/revoke fixture；`search_knowledge` shared scope 回归 | `wika.org_share.created`、`wika.org_share.revoked` |
