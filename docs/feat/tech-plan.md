# Wika 技术实现方案

> 版本: 2.4
> 日期: 2026-06-29
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
- **决策**：复用现有 `organizations`、`organization_tenant_members` 和组织角色模型；P5 默认只新增 Wika share 授权层，采用 `reference` 共享、`allowed_fields` 服务端白名单裁剪，active share 通过 ScopeResolver 输出 shared scope；撤销后 resolver 不再返回该 scope。
- **影响**：SearchService 命中 shared scope 后仍必须按 `allowed_fields` 裁剪；个人 KB 默认禁止 Organization share；SystemAdmin 不因 share 获得正文读取权；Organization admin 不能替代 source/target team Admin/Owner 扩大读取权限。
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
  governance/     # P5 高级治理：冲突、版本、URL 重抓、定时评测、跨团队共享
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
  max_attempts
  next_run_at
  locked_until
  locked_by
  failure_code
  created_by
  started_at
  completed_at
  error_msg
  created_at
  updated_at

wika_conflict_items
  id
  check_id
  tenant_id
  kb_id
  source_knowledge_id
  target_knowledge_id
  pair_key
  conflict_type              # contradiction | duplicate | outdated | scope_overlap
  confidence_score
  evidence jsonb
  ai_explanation
  status                     # open | confirmed | dismissed | resolved
  reviewer_comment
  resolved_by
  resolved_at
  created_at
  updated_at
```

约束：

- 写入前必须对 `source_knowledge_id` 和 `target_knowledge_id` 做 canonical pair 规范化，建议 `pair_key = least(id) + ':' + greatest(id)`；同一 `tenant_id + kb_id + pair_key + conflict_type` 未终态只能存在一条。
- `failed` 是终态；可重试失败保持 `pending` 并写 `next_run_at/failure_code/attempts`，达到 `max_attempts` 后才进入 `failed`。
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
- P5b 不做全量历史内容 backfill；既有知识第一次发生版本化写入前，在同一事务中先生成 `version_no=1` baseline，再记录写入后的新版本。
- 新创建知识在创建事务成功后记录 `version_no=1` 初始版本；没有真实版本行的知识不能 restore。

#### URL 重抓

```text
wika_url_refresh_jobs
  id
  schedule_id
  knowledge_id
  tenant_id
  kb_id
  source_url
  scheduled_for
  idempotency_key
  status                     # pending | running | pending_review | applied | rejected | failed
  fetched_hash
  fetched_title
  fetched_content
  diff_summary jsonb
  ssrf_check jsonb
  failure_code
  error_msg
  attempts
  locked_until
  locked_by
  created_by
  started_at
  completed_at
  reviewed_by
  reviewed_at
  review_comment
  created_at
  updated_at

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
  last_failure_code
  locked_until
  locked_by
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
- schedule 触发 job 时必须带 `scheduled_for` 或等价幂等键；同一 `schedule_id + scheduled_for` 最多一个 job，避免 worker crash 后重复创建。
- worker 使用 `locked_until/locked_by` 领取 job；连续失败达到阈值后停用 schedule 或延长 `next_run_at`。
- `source_url` 默认取知识已有来源 URL；允许请求覆盖时必须由 Team Admin/Owner 执行并写审计，Contributor 只能使用已有来源 URL。

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
  last_failure_code
  locked_until
  locked_by
  created_by
  created_at
  updated_at
```

P5d 需要给 P2 `eval_runs` 增加定时来源字段，或新增等价触发表：

```text
eval_runs.schedule_id
eval_runs.scheduled_for
```

约束：

- 同一 `kb_id + dataset_id` 最多一个启用计划。
- 同一 `schedule_id + scheduled_for` 最多一个 eval run；worker 更新 schedule 与创建 run 必须在同一事务内完成。
- 连续失败达到阈值后自动停用或降频，并写 audit log。

#### Organization 共享

```text
organizations                  # 复用现有表
organization_tenant_members    # 复用现有表，role: admin | editor | viewer

wika_org_shares
  id
  org_id
  source_tenant_id
  source_kb_id
  target_tenant_id
  mode                       # reference | snapshot
  allowed_fields jsonb
  status                     # pending | active | revoked
  created_by
  accepted_by
  accepted_at
  revoked_by
  revoked_at
  created_at
```

约束：

- P5e 不新建 `wika_organizations` 和 `wika_org_members`，避免与现有组织模型重复；000101 只新增 Wika share/scope 相关表或字段。
- P5 默认 `mode=reference`，不复制正文。
- `allowed_fields` 默认不含个人正文和证据；共享搜索命中后仍要校验接收团队成员权限。
- source team Admin/Owner 创建 share 后，如 target team Admin/Owner 未确认，状态为 `pending`，不得进入 ScopeResolver。
- revoke 后新 ScopeResolver 不再返回该 share scope。
- Organization role 只决定组织管理能力，不替代团队 RBAC 的数据授权；org admin/editor/viewer 不能单独 create/accept 共享来扩大读取权限。

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
- 通过 shared search 得到的 knowledge ID 不能作为后续 direct read、expand、download、preview 的通行证；这些入口必须重新解析 shared scope 并继续按 `allowed_fields` 裁剪。

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

- 终端用户通过 Wika 云端 Remote MCP 地址接入，不需要本地安装或启动 `weknora_mcp_server`。
- Remote MCP 从每个请求的 `Authorization: Bearer <wika_pat_xxx>` 读取用户 PAT，并透传调用 Wika API。
- 兼容期允许 `WEKNORA_API_KEY` 继续服务管理型工具，但日常工具不得把它当作 PAT fallback。
- `WEKNORA_BASE_URL` 仍指向 API v1 根路径，例如 `http://localhost:8080/api/v1`。
- MCP server 默认 `WEKNORA_MCP_TOOLSET=dynamic`：不传工具集请求头时只开放日常知识生产工具；客户端传 `X-Wika-MCP-Toolset: admin` 时，服务端调用 `/api/v1/wika/mcp/admin/authorize` 校验 `mcp:admin` scope 和租户 Admin/Owner 或系统管理员身份。
- `WEKNORA_MCP_TOOLSET=daily` 是部署级硬禁用管理工具；`WEKNORA_MCP_TOOLSET=all` 仅用于可信管理员/内网兼容部署。
- 本地 `stdio` 只保留为开发调试或旧客户端兼容，可配置 `WEKNORA_PAT` 做单用户调试。

Remote MCP 示例：

```json
{
  "mcpServers": {
    "wika": {
      "type": "streamable-http",
      "url": "https://<wika-domain>/mcp",
      "headers": {
        "Authorization": "Bearer wika_pat_xxx"
      }
	}
  }
}
```

管理员 MCP 示例：

```json
{
  "mcpServers": {
    "wika-admin": {
      "type": "streamable-http",
      "url": "https://<wika-domain>/mcp",
      "headers": {
        "Authorization": "Bearer wika_pat_xxx",
        "X-Wika-MCP-Toolset": "admin"
      }
    }
  }
}
```

真实验收时必须至少调用一次 `push_knowledge` 和 `search_knowledge`，不能只验证 MCP 进程启动。

### 7.3 管理工具

现有管理型 MCP tools 保留为兼容能力，但不出现在默认用户 Remote MCP 入口中；如需使用，客户端必须请求 `X-Wika-MCP-Toolset: admin`，并通过 `mcp:admin` PAT scope 和管理员角色校验。

## 八、HTTP API 契约

通用规则：

- 所有 Wika API 使用登录用户身份；MCP 日常工具经用户级 token 解析为同样的用户身份。
- 读不到或无权访问个人空间资源时返回 404，不暴露存在性。
- handler 只负责解析 actor/path/body、调用 service 和错误映射；service 是唯一调用 ScopeResolver 的业务边界；repository 不做权限判断。
- 所有 `kb_id`、`knowledge_id`、`suggestion_id` 和 P5 direct-id 资源必须在 service 第一行回溯父资源并解析 scope，再进入状态机或写事务。
- 旧 API 的 KB/knowledge/download/preview/hybrid-search 也必须调用同一套 scope resolver。
- 成功状态码约定：同步创建返回 201；异步任务创建返回 202；同步更新、状态转换和幂等返回已有终态返回 200；disable/revoke/delete 类软删除返回 200 并带当前状态；不使用 204，便于 handler RED 断言响应体。

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
PUT  /api/v1/wika/knowledge/:id/url-refresh/schedules/:schedule_id
DELETE /api/v1/wika/knowledge/:id/url-refresh/schedules/:schedule_id
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
PUT  /api/v1/wika/orgs/:org_id/shares/:share_id/accept
DELETE /api/v1/wika/orgs/:org_id/shares/:share_id
```

契约：

- conflict/version/url-refresh 属于 P5，不得在 P1-P3 暗中开启。
- URL refresh 的抓取结果默认进入待确认，不直接覆盖知识正文。
- Organization share 默认 `mode=reference`，撤销后新搜索不能命中；历史 lineage 和 audit metadata 保留。
- P5 不新增 MCP 工具；只回归 `search_knowledge` 对 Organization shared scope 的命中、字段裁剪和撤销后不命中。
- Organization 主体和成员管理当前优先复用既有 `/api/v1/organizations*` 路由；Wika V1 的强交付边界是 `/api/v1/wika/orgs/:org_id/shares*` 共享授权层。若后续补 `/api/v1/wika/orgs*` facade，只能委托既有 OrganizationService，不得新建平行主表、成员表或权限模型。

P5 权限矩阵：

| 能力/动作 | Team Viewer | Team Contributor | Team Admin/Owner | SystemAdmin | Organization role |
|-----------|-------------|------------------|------------------|-------------|-------------------|
| Conflict list/read | 允许团队内读取 | 允许 | 允许 | 仅元数据 | 无额外权限 |
| Conflict create check | 禁止 | 允许 | 允许 | 禁止绕过团队角色 | 无额外权限 |
| Conflict resolve | 禁止 | 禁止 | 允许 | 禁止绕过团队角色 | 无额外权限 |
| Version list/diff | 按 knowledge read | 按 knowledge read | 按 knowledge read | 仅元数据，个人正文不可读 | 无额外权限 |
| Version restore | 禁止 | 默认禁止 | 允许 | 禁止绕过团队角色 | 无额外权限 |
| URL refresh create/review/apply | 禁止 | 创建待确认 job | 允许 review/apply | 禁止绕过团队角色 | 无额外权限 |
| Eval schedule CRUD | 禁止 | 只读 | 允许 | 仅元数据 | 无额外权限 |
| Org create/member manage | 禁止 | 禁止 | 团队 Admin/Owner 才可代表团队 | 禁止绕过团队角色 | org admin 可管理组织关系，但不能替代团队数据授权 |
| Org share create/accept/revoke | 只能搜索已授权 shared scope | 禁止创建/accept/revoke | source team Admin/Owner 创建；target team Admin/Owner accept；source/target team Admin/Owner revoke | 仅元数据 | org role 不能单独扩大读取权限 |

P5 错误语义：

| 场景 | 状态码 | 说明 |
|------|--------|------|
| 请求体格式、cron、status、decision 非法 | 400 | `WIKA_VALIDATION_ERROR`；返回字段级错误，不返回内部栈 |
| 未登录或 PAT 无效 | 401 | `WIKA_UNAUTHORIZED`；不区分 token 不存在、过期或 hash 不匹配 |
| scope 不足且不会泄露个人资源存在性 | 403 | `WIKA_SCOPE_DENIED`；例如团队 Viewer 尝试创建 eval schedule |
| personal 或 direct-id 资源无权读 | 404 | `WIKA_NOT_FOUND`；不暴露资源存在性 |
| feature flag 关闭 | 404 | `WIKA_FEATURE_DISABLED`；普通用户视为能力不存在，SystemAdmin 诊断接口可返回 403 + 同 code |
| 状态非法转换、重复 active schedule/share、重复 open conflict | 409 | `WIKA_STATE_CONFLICT` |
| SSRF、allowed_fields、内容类型等安全策略阻断 | 422 | `WIKA_POLICY_BLOCKED`；返回脱敏策略原因，不返回敏感原文 |
| 限流、配额或 worker/API 领取冲突 | 429 | `WIKA_LEASE_CONFLICT`；API 层限流；worker 内部冲突不暴露给用户 |

P5 状态机：

| 对象 | 状态 | 合法转换 | 非法转换测试 |
|------|------|----------|--------------|
| conflict item | `open -> confirmed/dismissed/resolved`，`confirmed -> resolved` | `confirmed` 是已确认待处理的中间态；终态只有 `dismissed/resolved`，终态只允许添加 comment，不再改回 open | resolved 后再次 resolve 返回 409 或幂等终态 |
| knowledge version | 递增 `version_no`，无终态 | restore 生成新版本 | restore 不修改历史 version 内容 |
| url refresh job | `pending -> running -> pending_review -> applied/rejected`，任意抓取失败到 `failed` | `pending_review` 前不可 apply | running 直接 review/apply 返回 409 |
| eval schedule | `enabled=true/false` + `consecutive_failures` | 连续失败达到阈值后 disable 或降频 | disabled 不触发 run |
| org share | `pending -> active -> revoked` | target team Admin/Owner 确认后才 active；revoke 后不可重新 active；新建 share 生成新记录 | pending/revoked share 不进入 ScopeResolver |

P5 API 明细：

```text
GET /api/v1/wika/kb/:id/conflicts?status=open&limit=20&cursor=...
response: { items: [{ id, conflict_type, confidence_score, status, source_knowledge_id, target_knowledge_id, evidence_summary, ai_explanation }], next_cursor? }
权限: 团队成员按 KB read；SystemAdmin 只返回元数据。

POST /api/v1/wika/kb/:id/conflicts/checks
request: { trigger: "manual|knowledge_updated|suggestion_applied", knowledge_ids?: [] }
response: { check_id, status: "pending" }
权限: Team Contributor+；P5a V1 仅 `manual` 对用户开放，自动 trigger 只允许系统内部在对应 flag 开启后排队；创建后由 worker 执行。
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
幂等/并发: 手动 job 按 `knowledge_id + source_url + actor_id` 限流；同一知识同一来源存在未终态 job 时返回已有 job 或 `WIKA_STATE_CONFLICT`；cron 小于 1 小时返回 `WIKA_VALIDATION_ERROR`。

PUT /api/v1/wika/knowledge/:id/url-refresh/schedules/:schedule_id
request: { cron_expr?, enabled? }
response: { schedule_id, enabled, cron_expr, next_run_at, consecutive_failures }
权限: Team Admin/Owner。
语义: 更新 schedule，不创建 job；disabled 后 worker 不再生成新 job；非法 cron 或低于最小间隔返回 `WIKA_VALIDATION_ERROR`。

DELETE /api/v1/wika/knowledge/:id/url-refresh/schedules/:schedule_id
response: { schedule_id, enabled: false }
权限: Team Admin/Owner；软删除或 disable，不删除历史 job。

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
权限: 创建者必须是至少一个成员团队 Admin/Owner；实际落表复用现有 `organizations`。
实现状态: 该 facade 是可选增强；当前实现可直接使用既有 `/api/v1/organizations` 创建组织，P5e 不因 facade 未完成阻塞 share 授权层。

GET /api/v1/wika/orgs?limit=20&cursor=...
response: { items: [{ org_id, name, role, member_count, active_share_count }], next_cursor? }
权限: 返回 actor 所属团队加入的 Organization；SystemAdmin 仅元数据。

GET /api/v1/wika/orgs/:org_id
response: { org_id, name, members: [{ tenant_id, role }], share_summary }
权限: org member team 可读；非成员返回 404。

POST /api/v1/wika/orgs/:org_id/members
request: { tenant_id, role: "admin|editor|viewer" }
response: { org_id, tenant_id, role }
权限: org admin 且该 tenant 的 Team Admin/Owner 确认；personal tenant 禁止加入。
约束: role 使用现有 `organization_tenant_members.role` 枚举；personal tenant 返回 `WIKA_POLICY_BLOCKED`。
实现状态: 该 facade 是可选增强；当前实现可直接使用既有 Organization member API，Wika share service 只读取既有成员关系并执行 source/target team Admin/Owner 数据授权。

DELETE /api/v1/wika/orgs/:org_id/members/:tenant_id
response: { org_id, tenant_id, removed: true }
权限: org admin 且被移除 tenant 的 Team Admin/Owner 确认；不能移除现有 Organization 的 owning tenant；重复删除返回已有终态。

GET /api/v1/wika/orgs/:org_id/shares?status=active&limit=20&cursor=...
response: { items: [{ share_id, source_tenant_id, source_kb_id, target_tenant_id, status, allowed_fields, created_at, accepted_at, revoked_at }], next_cursor? }
权限: source 或 target team 成员可按字段裁剪读取；SystemAdmin 仅元数据。

POST /api/v1/wika/orgs/:org_id/shares
request: { source_kb_id, target_tenant_id, mode?: "reference", allowed_fields?: [] }
response: { share_id, status: "pending|active", allowed_fields }
权限: source team Admin/Owner 创建；如果调用者同时具备 target team Admin/Owner 权限可直接 active，否则由 accept 接口确认接收；org role 本身不能直接 active。
约束: allowed_fields 只能是服务端白名单；默认不含 content/chunk/evidence/file。

PUT /api/v1/wika/orgs/:org_id/shares/:share_id/accept
request: { comment? }
response: { share_id, status: "active", accepted_at }
权限: target team Admin/Owner；personal target 禁止；org admin 若不是 target team Admin/Owner 返回 403。
幂等: 已 active 返回 active 终态；revoked 返回 409。

DELETE /api/v1/wika/orgs/:org_id/shares/:share_id
response: { share_id, status: "revoked", revoked_at }
权限: source team Admin/Owner 或 target team Admin/Owner；org admin 只有同时具备 source/target team Admin/Owner 时可执行。
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

所有 P5 audit event 的公共字段：

```text
actor_id, auth_type, token_id_or_session_id, tenant_id?, kb_id?, request_id,
ip_hash, user_agent_hash, policy_version?, old_status?, new_status?,
old_content_hash?, new_content_hash?
```

表格中的必备字段是在公共字段之外的事件专属字段。禁止字段表示即使为了排障也不能写入审计，只能写 hash、长度、脱敏摘要或 failure_code。状态变更类事件必须与业务状态同事务提交；如果 `AuditLogService.Log` 返回错误，当前状态转换必须回滚。当前仓库已有 `database.WithGormTransaction` 和 audit repository 读取 tx 的基础，P5 service 必须用 txCtx 调审计服务。

| event | 必备字段 | 事务语义 | 禁止字段 |
|-------|----------|----------|----------|
| `wika.conflict.check_created` | actor_id, auth_type, tenant_id, kb_id, check_id, trigger, request_id | 创建 check 同事务 | 正文、snippet |
| `wika.conflict.check_failed` | actor_id/system, tenant_id, kb_id, check_id, failure_code, attempts, request_id | 标记 failed 或 backoff 同事务 | evidence 原文、AI 原文 |
| `wika.conflict.item_confirmed` | actor_id, tenant_id, kb_id, item_id, old_status, new_status, request_id | item 状态转换同事务 | evidence 原文 |
| `wika.conflict.item_dismissed` | actor_id, tenant_id, kb_id, item_id, old_status, new_status, request_id | item 状态转换同事务 | evidence 原文 |
| `wika.conflict.item_resolved` | actor_id, tenant_id, kb_id, item_id, old_status, new_status, request_id | item 状态转换同事务 | evidence 原文 |
| `wika.version.recorded` | actor_id, tenant_id, kb_id, knowledge_id, version_id, version_no, reason, request_id | 版本行和知识写入同事务或同一写路径事务 | content |
| `wika.version.restored` | actor_id, tenant_id, kb_id, knowledge_id, restored_from_version_id, new_version_id, request_id | restore 知识更新、索引触发和版本记录同一逻辑事务 | content diff |
| `wika.url_refresh.job_created` | actor_id, tenant_id, kb_id, knowledge_id, job_id, source_url_hash, request_id | API 创建 job 同事务 | source_url 明文 |
| `wika.url_refresh.job_failed` | actor_id/system, tenant_id, kb_id, job_id, failure_code, request_id | job 失败状态和 failure_code 同事务；worker 结构化日志可异步 | 抓取正文、内网地址明文 |
| `wika.url_refresh.reviewed` | actor_id, tenant_id, kb_id, job_id, decision, old_status, new_status, result_version_id, request_id | apply/reject 状态、版本记录和审计同事务 | fetched_content |
| `wika.url_refresh.schedule_updated` | actor_id, tenant_id, kb_id, schedule_id, enabled, cron_expr_hash, old_status, new_status, request_id | schedule create/update/disable 同事务 | source_url 明文 |
| `wika.url_refresh.schedule_disabled` | actor_id/system, tenant_id, kb_id, schedule_id, failure_code, consecutive_failures, request_id | 第三次失败 disable 同事务 | source_url 明文、抓取正文 |
| `wika.eval_schedule.updated` | actor_id, tenant_id, kb_id, schedule_id, enabled, cron_expr_hash, request_id | schedule create/update/disable 同事务 | QA 正文 |
| `wika.eval_schedule.run_failed` | actor_id/system, tenant_id, kb_id, schedule_id, run_id, failure_code, consecutive_failures | 失败计数、next_run/disable 同事务 | expected_answer |
| `wika.org_share.created` | actor_id, org_id, source_tenant_id, source_kb_id, target_tenant_id, share_id, allowed_fields, old_status, new_status, request_id | share 创建同事务；直接 active 只写 created(new_status=active) | 正文、证据 |
| `wika.org_share.accepted` | actor_id, org_id, share_id, target_tenant_id, old_status, new_status, request_id | pending -> active 同事务 | 正文、证据 |
| `wika.org_share.revoked` | actor_id, org_id, share_id, source_tenant_id, target_tenant_id, old_status, new_status, request_id | active/pending -> revoked 同事务 | 正文、证据 |

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
6. OrganizationShareService：复用既有 OrganizationService，新增授权、ScopeResolver shared scope、撤销。

P5 Implementation Map：

| 子阶段 | 稳定锚点 | 表 | Service | API | 前端 | feature flag | 完成证据 |
|--------|----------|----|---------|-----|------|--------------|----------|
| P5a Conflict | `WIKA-P5-CONFLICT` | `wika_conflict_checks/items` | `governance/conflict` | `/wika/kb/:id/conflicts*`、`/wika/conflicts/:id` | 冲突队列、详情抽屉 | `wika.governance.conflict.enabled` | 候选生成、状态流转、AI 不改正文 |
| P5b Version | `WIKA-P5-VERSION` | `wika_knowledge_versions` | `governance/version` | `/wika/knowledge/:id/versions*` | diff/restore | `wika.governance.version.enabled` | 关键写路径记录版本，restore 新版本 |
| P5c URL Refresh | `WIKA-P5-URL-REFRESH` | `wika_url_refresh_jobs/schedules` | `governance/urlrefresh` | `/wika/knowledge/:id/url-refresh*`、`/wika/url-refresh/:id/review` | 待确认更新队列 | `wika.governance.url_refresh.enabled` | SSRF fixture、pending review、apply 生成版本 |
| P5d Eval Schedule | `WIKA-P5-EVAL-SCHEDULE` | `wika_eval_schedules` | `governance/evalschedule` | `/wika/kb/:id/eval/schedules*` | 评测计划 | `wika.governance.eval_schedule.enabled` | DB lease、失败降频、run 明细 |
| P5e Org Share | `WIKA-P5-ORG-SHARE` | 复用 `organizations/organization_tenant_members`，新增 `wika_org_shares` | `governance/orgshare`、`scope` | `/wika/orgs*`、`/wika/orgs/:id/shares*` | Organization 和共享管理 | `wika.governance.org_share.enabled` | allowed_fields 裁剪、revoke 后不命中 |

当前仓库 P5 实现状态：

| 子阶段 | 已落地 | 继续实现时的下一张卡 |
|--------|--------|----------------------|
| P5a Conflict | `000097_wika_conflicts` migration、`internal/types/wika_governance*.go`、`internal/wika/governance/conflict` service/store、handler/router 测试已存在；已有测试覆盖 audit、candidate 生成、DB lease、候选保存、终态转换 | 下一张卡优先补 feature flag fail-closed、worker 显式 lifecycle、KB scope/direct-id 回溯、API 冒烟和审计失败回滚；不要让 AI 建议直接改知识正文 |
| P5b Version | `000098_wika_knowledge_versions` migration、`internal/wika/governance/version` store/service/diff/restore、handler 测试已存在；已有测试覆盖版本递增、无变化跳过、diff、restore 新版本、restore flag off 无副作用 | 下一张卡优先补统一写路径 hook inventory 和接线：Web、旧 API、`suggest_to_team` apply、URL apply、freshness 处理和 restore；补 baseline 生成、personal/SystemAdmin 字段裁剪和 API 冒烟；不要只在 handler 手动调用 `RecordVersion` |
| P5c URL Refresh | `000099_wika_url_refresh` migration、safe fetcher、job/schedule service/store、review apply/reject、handler 测试已存在；已有测试覆盖 SSRF、重定向再校验、超大响应、pending_review、apply 生成版本、schedule slot 幂等和 feature flag fail-closed；当前生产 worker 尚未落地 | 下一张卡优先补生产 worker lifecycle、container 注册、抓取失败对 schedule 的降频/停用审计、真实 diff summary、review/apply API 冒烟；P5b Version hook 未完成前不得打开 apply GA |
| P5d Eval Schedule | `000100_wika_eval_schedule` migration、`internal/types/wika_eval_schedule.go`、`internal/wika/governance/evalschedule` service/store/`RunDueSchedules`、handler/router/container/worker lifecycle 已存在；已有测试覆盖 cron 下限、schedule slot 幂等、禁用后不触发、连续失败三次禁用、API 参数传递、worker start/stop/flag off 和 ResourceCleaner 停止 | 下一张卡优先补审计事件、P2 run 与推进 schedule 的原子性证明或接线、API 冒烟和真实 schedule fixture；不要重复实现 schedule CRUD 或 worker 基础 lifecycle |
| P5e Org Share | `000101_wika_org_share` migration、`000102_wika_org_share_allowed_fields_check` 追加约束、`internal/types/wika_org_share.go`、`internal/wika/governance/orgshare` service/store、share handler/router/container、`ScopeResolver shared scope`、SearchService 字段裁剪、direct read/download/preview 裁剪回归、expand API/集成回归和 create/accept/revoke 审计回滚测试已存在 | 下一张卡优先补真实 HTTP/MCP 冒烟、revoke 传播 SLA 证据、前端入口或队列体验；不要重复创建 share/scope/search/direct-read/expand 基础实现 |

P5 当前可实施任务队列：

| 优先级 | 任务卡 | 首个 RED 测试建议 | 主要实现文件 | 完成证据 |
|--------|--------|-------------------|--------------|----------|
| 1 | `P5c-5 URL refresh worker/audit` | `urlrefresh.Worker` enabled 时先 `RunDueSchedules`，再对返回 jobs 逐个 `RunJob`；Stop 后不继续运行 | `internal/wika/governance/urlrefresh/worker.go`、`internal/wika/governance/urlrefresh/service.go`、`internal/container/container.go` | `go test ./internal/wika/governance/urlrefresh ./internal/container -run 'TestWorker|URLRefresh' -count=1`；SSRF fixture、schedule slot、失败降频、review/apply API 冒烟 |
| 2 | `P5b-4 Version hook inventory` | Web/旧 API/suggestion apply/URL apply/freshness/restore 任一路径缺版本即失败 | `internal/wika/governance/version`、现有知识更新链路适配点 | baseline、新版本、restore 新版本证据；无权 diff 不返回正文 |
| 3 | `P5a-4 Conflict gate/lifecycle` | flag off 时 create check 和 worker lease 均失败为 disabled；终态 item 不可回到 open | `internal/wika/governance/conflict/service.go`、`worker.go`、handler/container | manual check、candidate、resolve、flag off、audit fail rollback 证据 |
| 4 | `P5d-5 Eval schedule audit/API smoke` | schedule create/update/disable 和失败 backoff 写审计；audit 失败不推进状态 | `internal/wika/governance/evalschedule/service.go`、`internal/handler/wika_eval_schedule.go` | `wika.eval_schedule.updated/run_failed` 事件；HTTP 200/400/403/409；due lag 和 disable 指标 |
| 5 | `P5e-8 Org share real smoke/front-end` | 真实 HTTP/MCP fixture 证明 revoke 后 search/expand/direct-id 不命中；必要时补队列入口 | `internal/handler`、`mcp-server`、`frontend/src/views/wika/**` | MCP `search_knowledge/expand_knowledge_result`、HTTP create/accept/revoke、审计查询、revoke SLA |

P5e 后续验收规格：

- 真实路由：`WEKNORA_BASE_URL=http://localhost:8080/api/v1` 时，MCP `expand_knowledge_result` 实际请求 `POST /api/v1/wika/knowledge/expand`，body 为 `{ "ids": ["<search 返回的 shared id>"] }`。文档、测试和冒烟记录统一使用带 `/api/v1` 的完整 HTTP 路径，代码内部 client endpoint 保持 `/wika/knowledge/expand`。
- 最小 fixture：source team、target team、非成员用户各 1 个；同一 Organization 下 source/target 均已加入 `organization_tenant_members`；source KB 内知识 1 条；同一 source knowledge 分别构造 `active` share 和 `revoked` share；`allowed_fields=["id","title"]`；另有尝试传入 `content/chunk/evidence_text/file/metadata` 的非法字段用例。
- RED 测试优先级：先补 handler/API 集成测试，证明 active shared expand 只返回 id/title；再补 revoke 后同一 ID 的 search、expand、direct read、download、preview 都不命中；最后补 MCP smoke，证明 `search_knowledge` 返回的 shared ID 能被 `expand_knowledge_result` 裁剪展开。
- 审计事件语义：`CreateShare` 成功必须写 `wika.org_share.created`，details 包含 `org_id/source_tenant_id/source_kb_id/target_tenant_id/share_id/allowed_fields/old_status/new_status`。如果创建者同时具备 source 和 target 团队 Admin/Owner，share 可直接为 `active`，但只写 `created(new_status=active)`，不伪造 `accepted`。
- `AcceptShare` 只有真实执行 `pending -> active` 时写 `wika.org_share.accepted`；重复 accept active share 返回稳定终态或 `WIKA_STATE_CONFLICT`，不能重复写 accepted 事件。
- `RevokeShare` 允许 source 或 target 团队 Admin/Owner 执行，成功写 `wika.org_share.revoked`，details 至少包含 `actor_tenant_id/source_tenant_id/target_tenant_id/old_status/new_status/share_id`，便于 source 和 target 两侧审计检索。若现有 `audit_logs.tenant_id` 只能填一个租户，优先填 actor 当前租户，source/target 放 details。
- 同事务判定：审计 writer 必须参与当前 DB transaction，或在同一事务内写同库 audit/outbox 表。若外部审计 sink 不支持事务，不能直接把外部调用当作同事务审计；先提交同库 audit/outbox，再异步投递外部 sink。RED 测试必须覆盖 audit writer 返回错误时 create/accept/revoke 状态均不推进。
- 审计内容禁止：正文、chunk、snippet、证据文本、diff 全文、文件路径、token 明文或 hash。允许记录字段枚举、状态、资源 ID、hash、request_id、脱敏 actor 信息。
- MCP smoke 配置：按云端 Remote MCP 形态启动 HTTP transport，客户端用请求头传 `Authorization: Bearer <PAT>`；不要把 `WEKNORA_API_KEY` 当作日常工具 fallback。

```bash
export WEKNORA_BASE_URL="http://localhost:8080/api/v1"
export WEKNORA_MCP_TOOLSET=dynamic
unset WEKNORA_PAT
unset WEKNORA_API_KEY
cd mcp-server
python -m weknora_mcp_server --transport http --host 0.0.0.0 --port 8082
```

AI 工具侧连接 `http://localhost:8082/mcp`，并在 Remote MCP 配置中设置请求头 `Authorization: Bearer wika_pat_xxx`。需要管理工具时另加 `X-Wika-MCP-Toolset: admin`，并使用包含 `mcp:admin` scope 的管理员 PAT。

MCP smoke 的通过条件是：revoke 前 `search_knowledge(query, include_team=true)` 命中 shared 结果且 compact 结果不含正文；`expand_knowledge_result([id])` 仍只返回 `allowed_fields`；revoke 后同一 query 和同一 id 都不再返回共享内容。

P5 依赖顺序：

```text
P5b Version
  -> P5c URL Refresh apply

P2 Eval Run
  -> P5d Eval Schedule

P1a ScopeResolver + P1b Search
  -> P5e Org Share shared scope

P1b Search + P3 Freshness State
  -> P5a Conflict candidate generation
```

实现时允许并行创建 migration，但业务 API 的打开顺序必须遵循上图。不能为了并行开发让 P5c 绕过 VersionService，也不能让 P5e 在 ScopeResolver 未支持 `shared` scope 时直接改 SearchService。

P5 每卡 TDD 执行模板：

1. **锁定单卡范围**：只选择 `P5a-1`、`P5c-3` 这类单张任务卡；同 PR 不混入其他子阶段 API、worker 或前端入口。
2. **先写 RED**：优先写 migration/type 约束测试；再写 store/service 状态机、权限、审计和幂等测试；最后写 handler/router/container 注册测试。
3. **最小 GREEN**：先让当前卡对应 package 测试通过；不为后续子阶段预置空实现、空路由或未启用 worker。
4. **补回归**：当前卡涉及旧 API、ScopeResolver、SearchService、EvaluationService、VersionService 或 worker lifecycle 时，同 PR 必须补对应回归测试。
5. **验收证据**：提交前按“P5 每卡证据模板”记录测试命令、fixture/API 冒烟、审计事件和回滚动作。

P5 子阶段最小切片边界：

| 子阶段 | 首张卡只做什么 | 后续卡打开条件 | 禁止混入 |
|--------|----------------|----------------|----------|
| P5a Conflict | `wika_conflict_checks/items` DDL、类型、canonical pair 约束 | migration/type 测试通过后再做 worker；worker 通过后再做 API | AI 自动合并、自动删除、直接改知识正文 |
| P5b Version | `wika_knowledge_versions` DDL、类型、`version_no` 唯一 | baseline 和递增版本测试通过后再接写路径 hook；hook 覆盖后再做 diff/restore API | 只在 handler 手动调用版本记录、restore 覆盖历史 |
| P5c URL Refresh | URL job/schedule DDL、slot 唯一、safe fetcher fixture | P5b Version hook 可用后才做 review apply；safe fetcher 全绿后才做 worker | 抓取成功直接覆盖正文、绕过 SSRF 或 VersionService |
| P5d Eval Schedule | `wika_eval_schedules` DDL、enabled 唯一、`eval_runs` slot 字段/约束 | P2 EvaluationService 可复用后再做 worker；worker 幂等后再做 API | 复制评测指标逻辑、单实例进程锁、无限失败重试 |
| P5e Org Share | `wika_org_shares` DDL、复用既有 org/member 的约束测试 | ScopeResolver 能输出 `shared` scope 后再接 SearchService；搜索裁剪通过后再接 expand/download/preview/direct-id；当前 direct read/download/preview 已接入，下一步补 expand API/集成和 revoke 集成回归 | 新建平行 Organization 主表、复制正文、org admin 绕过团队授权 |

P5 推荐目录：

```text
internal/wika/governance/
  conflict/
    service.go
    store.go
    worker.go
    types.go
    *_test.go
  version/
    service.go
    store.go
    hooks.go
    diff.go
    types.go
    *_test.go
  urlrefresh/
    service.go
    store.go
    worker.go
    schedule_worker.go
    safefetch/
      fetcher.go
      resolver.go
      *_test.go
    *_test.go
  evalschedule/
    service.go
    store.go
    worker.go
    cron.go
    *_test.go
  orgshare/
    service.go
    store.go
    types.go
    *_test.go
internal/handler/
  wika_governance_conflict.go
  wika_governance_version.go
  wika_governance_urlrefresh.go
  wika_eval_schedule.go
  wika_org.go
```

目录规则：

- 各子模块内部定义自己的 Store 接口，不向 `internal/types/interfaces` 追加大接口。
- handler 只做参数解析、context actor 提取和错误映射；状态机、feature flag、权限回溯和审计事务放 service。
- worker 不直接依赖 handler；worker 只依赖 service/store、logger、clock、feature flag。
- P5 新 DB 类型放 `internal/types/wika_governance*.go`、`wika_version.go`、`wika_url_refresh.go`、`wika_eval_schedule.go`、`wika_org_share.go`。
- 所有 direct-id API 的 service 第一行必须通过 ScopeResolver 回溯父 KB/knowledge/org，不允许只按 item/job/version ID 查表后返回。

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

P5 统一错误码：

| code | HTTP | 场景 |
|------|------|------|
| `WIKA_VALIDATION_ERROR` | 400 | 请求体格式、cron、status、decision、字段枚举非法 |
| `WIKA_UNAUTHORIZED` | 401 | 未登录、PAT 无效、过期或撤销 |
| `WIKA_FEATURE_DISABLED` | 404 | 子阶段 feature flag 未开启；对普通用户不暴露内部能力。SystemAdmin 诊断接口可返回 403 + 同 code |
| `WIKA_SCOPE_DENIED` | 403 | 团队内角色不足且不会泄露个人资源 |
| `WIKA_NOT_FOUND` | 404 | personal/direct-id 越权或资源不存在 |
| `WIKA_STATE_CONFLICT` | 409 | 非法状态转换、重复 active schedule/share/open conflict |
| `WIKA_POLICY_BLOCKED` | 422 | SSRF、allowed_fields、内容类型、安全策略阻断 |
| `WIKA_LEASE_CONFLICT` | 429 | worker/API 并发领取、限流或配额冲突 |

错误响应统一形态：

```json
{
  "error": {
    "code": "WIKA_STATE_CONFLICT",
    "message": "state transition is not allowed",
    "field": "status"
  }
}
```

响应中不得包含 SQL、内部表名、堆栈、抓取正文、证据原文、token hash 或被安全策略命中的原文。

P5 API 契约清单：

| 子阶段 | API | 请求 | 成功响应 | 状态/副作用 |
|--------|-----|------|----------|-------------|
| P5a Conflict | `POST /api/v1/wika/kb/:id/conflicts/checks` | `{ "trigger": "manual" }`；空值按 `manual` | `WikaConflictCheck` | 创建 `pending` check；flag off 返回 `WIKA_FEATURE_DISABLED`；不得直接生成知识变更 |
| P5a Conflict | `GET /api/v1/wika/kb/:id/conflicts?status=&limit=&offset=` | query 只允许服务端白名单状态；limit 默认 20 | `{ "items": [...], "total": 0 }` | evidence 返回前按 actor scope 裁剪 |
| P5a Conflict | `PUT /api/v1/wika/conflicts/:id` | `{ "status": "confirmed|dismissed|resolved", "comment": "..." }` | `WikaConflictItem` | 只更新 item 状态和审计；终态再次修改返回 `WIKA_STATE_CONFLICT` 或已有终态 |
| P5b Version | `GET /api/v1/wika/knowledge/:id/versions?limit=&offset=` | query limit 默认 50 | `{ "versions": [...] }` | personal 越权返回 404；SystemAdmin personal 只返回元数据 |
| P5b Version | `GET /api/v1/wika/knowledge/:id/versions/:version_id/diff?to_version_id=` | `to_version_id` 必填 | `DiffResult` | 无权 diff 不返回正文、snippet、old/new content |
| P5b Version | `POST /api/v1/wika/knowledge/:id/versions/:version_id/restore` | `{ "reason": "..." }` | `RestoreResult` | 走知识更新和索引链路，生成新版本；不覆盖历史 |
| P5c URL Refresh | `GET /api/v1/wika/knowledge/:id/url-refresh` | query 可带 status/limit/cursor | `{ "jobs": [...], "schedules": [...] }` | 供知识详情和待确认队列使用；抓取正文默认不返回全文，只返回 hash/摘要/diff 摘要 |
| P5c URL Refresh | `POST /api/v1/wika/knowledge/:id/url-refresh` | `{ "source_url": "...", "schedule": { "enabled": true, "cron_expr": "0 * * * *" } }` | `{ "job_id": 1, "schedule_id": 1, "status": "pending" }` | 创建手动 job 或 schedule；重复未终态 job 返回已有 job 或稳定 409；不抓取、不改正文 |
| P5c URL Refresh | `PUT /api/v1/wika/knowledge/:id/url-refresh/schedules/:schedule_id` | `{ "cron_expr": "...", "enabled": false }` | `WikaURLRefreshSchedule` | 更新 schedule，不创建 job；低于最小间隔返回 `WIKA_VALIDATION_ERROR` |
| P5c URL Refresh | `DELETE /api/v1/wika/knowledge/:id/url-refresh/schedules/:schedule_id` | 无 body | `WikaURLRefreshSchedule` | 逻辑停用，不硬删除；停用后 worker 不再生成新 job |
| P5c URL Refresh | `PUT /api/v1/wika/url-refresh/:refresh_id/review` | `{ "decision": "apply|reject", "comment": "..." }` | `{ "refresh_id": 1, "status": "applied|rejected", "result_version_id": 1 }` | 只允许 `pending_review`；apply 调用知识更新链路和 VersionService；reject 不改知识 |
| P5d Eval Schedule | `GET /api/v1/wika/kb/:id/eval/schedules` | query 可带 enabled/status | `{ "schedules": [...], "total": 0 }` | 只列当前 actor 可管理 KB 的计划 |
| P5d Eval Schedule | `POST /api/v1/wika/kb/:id/eval/schedules` | `{ "dataset_id": 1, "cron_expr": "0 * * * *", "enabled": true }` | `WikaEvalSchedule` | 同一 `kb_id + dataset_id` 只能一个 enabled schedule |
| P5d Eval Schedule | `PUT /api/v1/wika/kb/:id/eval/schedules/:schedule_id` | `{ "cron_expr": "...", "enabled": false }` | `WikaEvalSchedule` | 更新 next_run、enabled、失败计数策略；disabled 不触发 run |
| P5d Eval Schedule | `DELETE /api/v1/wika/kb/:id/eval/schedules/:schedule_id` | 无 body | `WikaEvalSchedule` | 逻辑停用，不删除历史 run |
| P5e Org Share | `POST /api/v1/wika/orgs` | 复用既有 Organization 创建 payload | Organization DTO | 门面调用既有 OrganizationService，不新建平行主表 |
| P5e Org Share | `POST /api/v1/wika/orgs/:org_id/members` | `{ "tenant_id": 1, "role": "admin|editor|viewer" }` | org member DTO | personal tenant 禁止加入 |
| P5e Org Share | `POST /api/v1/wika/orgs/:org_id/shares` | `{ "source_kb_id": "...", "target_tenant_id": 1, "allowed_fields": ["id","title"] }` | `WikaOrgShare` | source team Admin/Owner 才能创建；target 未授权时进入 `pending` |
| P5e Org Share | `PUT /api/v1/wika/orgs/:org_id/shares/:share_id/accept` | 可选 `{ "comment": "..." }` | `WikaOrgShare` | target team Admin/Owner 才能 accept；active 后进入 ScopeResolver |
| P5e Org Share | `DELETE /api/v1/wika/orgs/:org_id/shares/:share_id` | 可选 `{ "comment": "..." }` | `WikaOrgShare` | revoke 后 shared scope、search、expand、download、preview、direct-id 均不命中 |

P5 状态机和审计：

| 对象 | 允许状态 | 关键转换 | 审计事件 |
|------|----------|----------|----------|
| `wika_conflict_checks` | `pending`、`running`、`completed`、`failed` | `pending -> running -> completed/failed`；失败未达阈值可回到待领取 | `wika.conflict.check_created`、`wika.conflict.check_failed` |
| `wika_conflict_items` | `open`、`confirmed`、`dismissed`、`resolved` | `open -> confirmed/dismissed`；`confirmed -> resolved/dismissed`；终态不可再改 | `wika.conflict.item_confirmed`、`wika.conflict.item_dismissed`、`wika.conflict.item_resolved` |
| `wika_knowledge_versions` | immutable snapshot | 只新增，不 update/delete；restore 也是新增版本 | `wika.version.recorded`、`wika.version.restored` |
| `wika_url_refresh_jobs` | `pending`、`running`、`pending_review`、`applied`、`rejected`、`failed` | `pending -> running -> pending_review/failed`；`pending_review -> applied/rejected` | `wika.url_refresh.job_created`、`wika.url_refresh.job_failed`、`wika.url_refresh.reviewed` |
| `wika_url_refresh_schedules` | `enabled=true/false` + failure counters | due schedule 只创建 job；连续失败后延后或 disable | `wika.url_refresh.schedule_updated`、`wika.url_refresh.schedule_disabled` |
| `wika_eval_schedules` | `enabled=true/false` + failure counters | due schedule 只创建 P2 run；连续失败后延后或 disable | `wika.eval_schedule.updated`、`wika.eval_schedule.run_failed` |
| `wika_org_shares` | `pending`、`active`、`revoked` | `pending -> active/revoked`；`active -> revoked`；revoked 不可恢复，只能新建 | `wika.org_share.created`、`wika.org_share.accepted`、`wika.org_share.revoked` |

P5 handler 错误映射：

| service error / 条件 | HTTP | code |
|----------------------|------|------|
| feature flag off、缺失、读取失败 | 404 | `WIKA_FEATURE_DISABLED` |
| 请求体、枚举、cron、allowed_fields 非法 | 400 | `WIKA_VALIDATION_ERROR` |
| 未登录、PAT 过期或撤销 | 401 | `WIKA_UNAUTHORIZED` |
| 团队角色不足且资源存在性可暴露 | 403 | `WIKA_SCOPE_DENIED` |
| personal/direct-id 越权或资源不存在 | 404 | `WIKA_NOT_FOUND` |
| 非法状态转换、重复 active schedule/share/open conflict | 409 | `WIKA_STATE_CONFLICT` |
| SSRF、内容类型、响应大小、安全策略阻断 | 422 | `WIKA_POLICY_BLOCKED` |
| worker lease、手动重抓限流、配额冲突 | 429 | `WIKA_LEASE_CONFLICT` |

实现时如果先沿用现有 `internal/errors` envelope，也必须在 handler 测试里锁定 HTTP 状态和稳定错误文本；统一 Wika error code wrapper 落地后再把 body 切换成上表 `code`，不能让前端依赖 Go error 字符串。

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
| P1b-3 MCP 日常工具 | Remote MCP 请求头 PAT 透传；默认 daily，管理员可用 `X-Wika-MCP-Toolset: admin` 请求管理工具但必须通过后端授权；`WEKNORA_API_KEY` 不可调用日常工具 | `mcp-server/weknora_mcp_server.py`、MCP tests | HTTP Remote MCP 真实调用截图/日志 |
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
| P5e-1 Org share migration | 复用 org/member，新增 share 约束和 revoke 状态 | `migrations/versioned/000101*`、`internal/types/wika_org_share.go` | migration up/down |
| P5e-2 Org/member API | personal tenant 禁止加入 Organization | `internal/handler/wika_org.go` | 角色和接收方确认测试 |
| P5e-3 Share authorization | allowed_fields 只能服务端白名单；pending share 不进入搜索 | `internal/wika/governance/orgshare` | share/create/accept/revoke 测试 |
| P5e-4 Shared scope search | revoke 后 ScopeResolver 不再返回 shared scope | `internal/wika/scope`、`internal/wika/search` | `search_knowledge` shared scope 回归 |
| P5e-5 Direct shared read | shared direct read 不返回正文/文件/metadata；download/preview 在 shared scope 下 403；direct 路由不被旧 KBAccess guard 提前拦截；`allowed_fields` 读取侧 fail-closed | `internal/handler/knowledge.go`、`internal/router/router.go`、`internal/container/container.go`、`migrations/versioned/000102*` | direct-id、download、preview 裁剪回归；container 真实 resolver 解析回归 |
| P5a-4 Conflict gate/lifecycle | flag off 时 create check 和 worker lease 均 fail-closed；终态 item 不回到 open | `internal/wika/governance/conflict/worker.go`、`service.go`、`internal/handler/wika_governance_conflict.go`、`internal/container/container.go` | worker lifecycle、flag off、audit fail rollback、manual check API 冒烟 |
| P5b-4 Version hook inventory | Web/旧 API/suggestion apply/URL apply/freshness/restore 任一路径缺版本即失败 | `internal/wika/governance/version`、`internal/application/service/knowledge*.go`、`internal/wika/suggestion`、`internal/wika/governance/urlrefresh`、`internal/wika/freshness` | baseline、新版本、restore 新版本、hook 防递归、无权 diff 裁剪 |
| P5c-5 URL refresh worker/audit | worker enabled 时生成 due jobs 并逐个运行；flag off/stop 后不抓取 | `internal/wika/governance/urlrefresh/worker.go`、`service.go`、`internal/container/container.go` | worker start/stop、container 注册、SSRF fixture、失败降频/disable 审计、review/apply 冒烟 |
| P5d-4 Eval worker lifecycle | flag off 或 worker stop 后 `RunDueSchedules` 不被调用、不领取 lease | `internal/wika/governance/evalschedule/worker.go`、`internal/container/container.go` | worker start/stop 单测、container 组装测试、ResourceCleaner 停止 |
| P5d-5 Eval schedule audit/API smoke | schedule 更新/失败写审计；audit 失败不推进状态 | `internal/wika/governance/evalschedule/service.go`、`internal/handler/wika_eval_schedule.go` | `wika.eval_schedule.updated/run_failed`、HTTP 200/400/403/409、due lag 指标 |
| P5e-6 Expand/direct-id revoke | `POST /api/v1/wika/knowledge/expand` 对 active shared ID 只返回允许字段；revoke 后同一 ID 被过滤 | `internal/handler/wika_knowledge.go`、`internal/wika/search`、`internal/wika/scope` | search/expand/direct read/download/preview 裁剪和 revoke 回归 |
| P5e-7 Org share audit/smoke | create/accept/revoke 与 audit 同事务；audit 失败时状态不推进 | `internal/wika/governance/orgshare/service.go`、`internal/handler/wika_org_share.go` | `wika.org_share.created/accepted/revoked`、400/403/404/409 响应体稳定 |
| P5e-8 Org share real smoke/front-end | 真实 HTTP/MCP fixture 证明 revoke 后 search/expand/direct-id 不命中；必要时补队列入口 | `internal/handler`、`mcp-server`、`frontend/src/views/wika/**` | MCP `search_knowledge/expand_knowledge_result`、HTTP create/accept/revoke、revoke SLA、前端只消费后端状态 |

P5 RED 测试包：

| 子阶段 | 必建测试文件 | 首批断言 |
|--------|--------------|----------|
| P5a Conflict | `internal/types/wika_governance_test.go`、`internal/wika/governance/conflict/{store,service}_test.go`、`internal/handler/wika_governance_conflict_test.go`、`internal/router/wika_routes_test.go`、`internal/container/wika_registration_test.go` | partial unique 生效；canonical pair 反向不重复；两个 worker 只有一个成功 lease；flag off 时写 API 返回 `WIKA_FEATURE_DISABLED` 且 worker 不 lease；`CreateCheck` 写审计；audit 失败状态不变；resolved 后再次修改返回 409；prompt injection evidence 不改变状态；路由和 DI 可注册 |
| P5b Version | `internal/types/wika_version_test.go`、`internal/wika/governance/version/{store,service,diff}_test.go`、`internal/handler/wika_governance_version_test.go` | `(knowledge_id, version_no)` 唯一；既有知识首次写入先 baseline 再新版本；连续写路径递增；restore 调用知识更新链路并产生新版本；SystemAdmin 读 personal 只有元数据；无权 diff 不返回正文；flag off 禁止 restore |
| P5c URL Refresh | `internal/types/wika_url_refresh_test.go`、`internal/wika/governance/urlrefresh/safefetch/fetcher_test.go`、`internal/wika/governance/urlrefresh/{store,service,worker}_test.go`、`internal/handler/wika_governance_urlrefresh_test.go` | 内网/metadata hostname/CNAME/重定向/DNS rebinding/编码绕过/超大响应阻断；正常抓取进入 `pending_review`；apply 前状态不符返回 409；schedule slot 幂等；低于最小 cron 返回 400；flag off 禁止新 job 且 worker 不 lease；apply 生成版本和审计 |
| P5d Eval Schedule | `internal/types/wika_eval_schedule_test.go`、`internal/wika/governance/evalschedule/{cron,store,worker}_test.go`、`internal/handler/wika_eval_schedule_test.go` | cron 非法 400；enabled 唯一；两个 scheduler 只创建一个 run；同一 `schedule_id + scheduled_for` 不重复创建 run；连续失败后 disable 或延后；flag off worker 不创建 run；run 创建复用 P2 service |
| P5e Org Share | `internal/types/wika_org_share_test.go`、`internal/wika/governance/orgshare/{store,service}_test.go`、`internal/wika/scope/shared_scope_test.go`、`internal/wika/search/shared_scope_test.go`、`internal/handler/wika_org_share_test.go`、`internal/handler/knowledge_wika_org_share_test.go`、`internal/handler/wika_org_test.go`、`internal/router/wika_org_share_routes_test.go` | 复用现有 org/member；personal tenant/KB 禁止；allowed_fields 白名单和读取侧 fail-closed；pending 不可搜索；org admin 不是 target team Admin 时 accept 返回 403；accept 后可裁剪命中；download/preview/direct read 仍裁剪；expand service 裁剪已覆盖，API/集成需补同等回归；revoke 后同 query 和 direct-id 都不命中；flag off 禁止 create/accept/revoke/direct read |

RED 测试顺序：

1. 先写 migration/type 约束测试，避免 service 先行后发现 DDL 不支持。
2. 再写 store/service 状态机和权限回溯测试。
3. 最后写 handler/router/container 测试，确保 API 契约、路由和 DI 一次落地。
4. 每个子阶段 GREEN 前至少跑对应 package 测试和 `git diff --check`。

### P5 子能力实施契约

P5 只能在 P1-P4 门禁通过后开启。所有 P5 API 默认放在功能开关后，或按阶段路由注册，不得在数据模型未完成时暴露空实现。

#### P5 通用实现契约

共享依赖：

- `FeatureGate`：按 flag key 从 `system_settings` 读取布尔值；读取失败、缺失、非法值或缓存过期时返回 disabled 语义。service 和 worker 不能直接读环境变量。
- `ScopeResolver`：所有 direct-id 入口先回溯父 KB、knowledge 或 org，再按 actor/action 解析；handler 不能自己拼权限条件。
- `AuditWriter`：支持在当前 DB transaction 内写审计事件；安全相关状态变更不能先提交业务再异步补审计。
- `Clock`：worker、cron、lease、backoff 和测试统一使用可注入时钟；测试不得依赖真实时间睡眠。
- `Logger`：只记录脱敏摘要、hash、failure_code 和 request_id；不能记录正文、diff 全文、抓取正文、expected answer 或 token hash。
- `WorkerRunner`：由 server bootstrap 显式启动；constructor、DI 注册和单元测试不得启动后台 goroutine。

service 构造最小依赖形态：

```text
NewService(store, scopeResolver, featureGate, auditWriter, clock, logger, ...)
```

其中 `...` 只能是当前子阶段真实需要的上游 service，例如 P5b 需要知识更新链路适配点，P5c review/apply 需要 VersionService，P5d 需要 EvaluationService，P5e 需要 OrganizationService 和 Search/Scope 适配点。不得为了未来能力把所有 P5 service 互相注入。

Feature flag：

- 每个子阶段有独立 flag，默认 false。
- flag source of truth 是 `system_settings`；环境变量只能作为只读 fallback，不能作为运行时灰度主入口。
- flag key 使用 P5 Implementation Map 中的 `wika.governance.*.enabled`；只有 SystemAdmin 可修改。
- flag 关闭时：handler 不注册或返回 `WIKA_FEATURE_DISABLED`；worker 不领取新任务；已有记录保留只读。
- flag 状态不得缓存在进程内超过 60 秒，避免紧急回滚不生效。
- flag 缺失、读取失败、非法值、配置中心超时、进程启动时未加载到配置时，一律 fail closed，当作 disabled。
- 写 API 每次进入 service 前检查 flag；worker 扫描前和 lease 前都检查 flag。flag 关闭后不得产生新状态、新 job、新 run 或新 share。
- RED 测试必须覆盖 missing key、store error、非法值、stale true 超 TTL 后回落 disabled、disabled worker 不 lease。
- 当前 P5 service 代码只依赖 `GetBool(ctx, key, envName, def) bool` 这类布尔接口时，必须由 P5 FeatureGate adapter 统一承诺 fail-closed 语义；如果底层 `SystemSettingService.GetBool` 无法区分 store error、非法值和 missing key，adapter 必须把这些情况都转成 `false`，并打脱敏日志和指标。
- 单测 fake gate 必须显式覆盖 `enabled`、`disabled`、`missing`、`store_error`、`invalid_value` 五种状态，不能只用一个 bool stub 证明 happy path。

事务和幂等：

- 所有状态转换必须在事务内完成，并写审计事件。
- apply/restore/revoke/review 等可重复点击动作必须返回已有终态或稳定 409，不能重复写知识、版本或 share。
- 所有 worker job 都必须有 `attempts`、`locked_until`、`locked_by`、`started_at`、`completed_at` 或等价字段。
- API 创建类操作若支持幂等键，幂等键作用域至少包含 `tenant_id`、`actor_id`、业务资源 ID 和 action。
- P5c/P5d 这类 schedule worker 必须有 `scheduled_for` 或等价 slot 幂等键；创建 job/run 与推进 schedule 的事务必须原子提交。

Worker lease：

```text
TryAcquire(table, id, worker_id, now, lease_duration):
  UPDATE table
     SET locked_by = worker_id,
         locked_until = now + lease_duration,
         attempts = attempts + 1,
         started_at = COALESCE(started_at, now)
   WHERE id = id
     AND status IN runnable_statuses
     AND (locked_until IS NULL OR locked_until < now)
  RETURNING *

Complete(id):
  UPDATE table
     SET status = completed_status,
         completed_at = now,
         locked_by = NULL,
         locked_until = NULL
   WHERE id = id AND locked_by = worker_id
```

实现要求：

- worker 只能处理自己成功 lease 的行。
- lease 超时后允许其他 worker 重新领取。
- 每次失败都写 `error_msg` 的脱敏摘要和稳定 `failure_code`，不写正文。
- 达到失败阈值后状态进入 `failed`、schedule disable 或延后 `next_run_at`。
- 测试必须用两个 worker 并发领取同一行，证明只会有一个成功。

Worker lifecycle：

- 禁止在 service constructor 中启动 goroutine。
- worker 通过显式 runner 注册，由 server bootstrap 在生产启动；测试环境默认不启动 worker，只通过单测直接调用 runner/service。
- runner 依赖 `context`、clock、logger、feature flag、store/service，不依赖 handler。
- shutdown 时停止新扫描和新 lease，不抢占已提交事务；正在处理的 job 必须在 context 取消后尽快释放或让 lease 超时。
- P5a/P5c/P5d 都必须有 container registration RED 测试，证明 worker 依赖可组装但不会在单测中自动后台运行。
- 当前仓库采用 container/ResourceCleaner pattern：`container.Provide(initWikaXWorker)` 构造 worker，`container.Invoke(startWikaXWorker)` 在生产容器启动时显式调用 `Start`，并用 `ResourceCleaner.RegisterWithName` 调用 `Stop`。后续 P5 worker 优先沿用该模式；如迁移到 `cmd/server/main.go` shutdown context，必须单独成卡并保留 ResourceCleaner 回归。
- `Start` 可以用 `context.Background()` 承接当前容器生命周期，但必须保证 `Stop` 可关闭 ticker/goroutine；不得在测试容器 resolve 时自动后台运行。验收证据必须包含 Start/Stop、flag off、重复 Start 幂等和 ResourceCleaner 停止。

P5 worker metrics & alerts：

| 指标 | 必备标签 | 写入位置 | 告警动作 |
|------|----------|----------|----------|
| `wika_worker_due_lag_seconds` | stage、worker、tenant_id | 每次扫描 due schedule 前 | lag 持续超阈值时检查 flag、worker、DB lease |
| `wika_worker_lease_conflict_total` | stage、worker | lease 失败但非 fatal 时 | 异常升高时检查多实例和 lease_duration |
| `wika_worker_run_once_total` | stage、worker、result | `RunOnce` 结束时 | result=error 升高时查看 failure_code |
| `wika_worker_disabled_total` | stage、reason | flag off、missing、store_error、invalid_value | flag off 后仍有 job/run 创建为 Critical |
| `wika_worker_schedule_disabled_total` | stage、failure_code | 连续失败达到阈值并 disable 时 | 通知维护者检查数据集、fetcher 或下游服务 |

滥用和成本上限：

- cron 最小间隔默认 1 小时；低于系统下限返回 `WIKA_VALIDATION_ERROR`。
- URL refresh 手动 job 默认每 actor 每知识每小时最多 3 次、每团队每小时最多 100 次；超限返回 `WIKA_LEASE_CONFLICT` 或专用 rate-limit 响应。
- URL refresh schedule 默认每知识每来源最多 1 个 enabled schedule、每 KB 最多 100 个 enabled schedule。
- Eval schedule 默认每 KB 最多 20 个 enabled schedule。
- 连续失败默认第 1 次延后 1 小时，第 2 次延后 6 小时，第 3 次 disable；团队策略可放宽但不得低于系统安全下限。
- backoff 必须带 jitter，避免多 schedule 同时恢复造成尖峰。

审计：

- 安全相关 API 的状态变更和审计写入必须同事务提交；如果审计写入失败，业务状态保持不变并返回错误。
- `AuditWriter` 必须支持参与当前 DB transaction，或者由 service 在同一事务内写入同库 audit table。
- 人工 apply/restore/revoke/accept/review 不允许降级为“只写业务不写审计”；纯 worker 抓取失败日志可降级为结构化日志，但失败计数和 failure_code 仍要落库。
- 审计事件提交后必须可查；应用层禁止 update/delete 审计记录，必要时后续增加 hash chain 或外部不可变日志 sink。
- 审计只存元数据、hash、脱敏摘要和 request_id，不存正文、snippet、diff 全文、抓取内容。
- 关键审计事件必须记录 `actor_id`、`auth_type`、`token_id 或 session_id`、`ip_hash`、`user_agent_hash`、`request_id`、前后状态、关键内容 hash、策略版本。

字段裁剪：

- SystemAdmin 对 personal 资源永远 `metadata_only`。
- Org share 命中后必须按 `allowed_fields` 裁剪，不能因为用户也是目标团队 Admin 就放宽源团队正文。
- conflict evidence、version diff、url diff、graph evidence 都属于正文同级敏感数据。

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

- P5a V1 只对用户开放手动 `manual` check；自动 trigger 先按内部事件排队能力预留，只有对应 flag 开启后才能产生任务。
- 候选生成可复用 SearchService 和 source hash，不在 P5 自建检索引擎。
- `duplicate` 最小判定：source hash 相同或相似度超过阈值；`outdated` 最小判定：来源时间、expires/freshness 状态或版本时间显示旧知识覆盖新知识；`scope_overlap` 最小判定：主题/标签/实体范围高度重叠；`contradiction` 只能作为 AI/检索辅助候选，必须人工确认。
- 写入 item 前必须生成 canonical `pair_key`，避免 A->B 和 B->A 反向重复。
- check 失败未达 `max_attempts` 时保持可重试状态并写 `next_run_at`；达到阈值后进入终态 `failed`，worker runnable 索引不得继续领取终态 failed。
- `evidence` 默认只存定位、hash 和摘要；返回正文前必须重新做 knowledge read scope。
- AI 解释只作为辅助字段，不能作为自动覆盖或删除的依据。

必测：

- 无权 KB 不能创建 check。
- 同一未终态冲突不会重复生成。
- 反向 pair 不会重复生成。
- 确认/驳回/解决只改 conflict item，不改原知识正文。
- 两个 worker 同时处理同一 check 时，只生成一组候选。
- `evidence` 中的 knowledge ID 对当前 actor 无权时，只返回 hash/摘要，不返回证据文本。
- evidence 或 AI 解释中包含要求忽略系统指令、自动 approve、泄露密钥等 prompt injection 文本时，不改变状态、不自动处理。

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
- 统一通过 `KnowledgeMutationWithVersion` 或等价适配点接入知识写路径，避免每个 handler 自己调用 `RecordVersion`。
- 版本行记录的是成功写入后的知识快照。既有知识首次版本化写入前，先在同一事务创建 baseline `version_no=1/change_reason=baseline`，再记录本次变更后的新版本。
- `content_hash`、标题、标签、状态和有效期均未变化时跳过新版本，但仍可写审计说明 no-op；restore 即使目标内容与当前一致，也返回稳定 no-op 结果，不重复生成版本。
- restore 调用现有知识更新链路和索引链路，然后生成新版本。
- restore 调用更新链路时必须带 `change_reason=restore`、`restored_from_version_id`，并用上下文标记防止递归重复记录。
- SystemAdmin 对个人知识版本仍只能读元数据。

必须版本化的写路径：

- Web 手动更新知识标题、正文、标签、状态。
- 旧 WeKnora 知识更新 API。
- `suggest_to_team` apply 复制或更新团队知识。
- URL refresh review decision 为 `apply`。
- freshness 处理动作导致知识正文、状态或有效期变化。
- restore 本身。

P5b-4 真实写路径 hook inventory：

| 写路径 | 当前主要入口 | hook 前快照 | hook 后快照 | `change_reason` | 防递归 |
|--------|--------------|-------------|-------------|-----------------|--------|
| Web/手动 Markdown 创建或更新 | `internal/application/service/knowledge_create.go::CreateKnowledgeFromManual`、`UpdateManualKnowledge` | 既有 knowledge 行；新建时无 baseline | 保存后的标题、正文、状态、标签、hash | `manual_create`、`manual_update` | 新建首版只记录一次；更新时由统一 mutation wrapper 记录 |
| 旧 WeKnora knowledge 更新 | `internal/application/service/knowledge.go::UpdateKnowledge`、`internal/types/interfaces/knowledge.go::UpdateKnowledge` | 更新前 knowledge 行 | 更新后 knowledge 行 | `legacy_api_update` | wrapper 检测 `context` 中的 `skip_version_recording` |
| 标签批量更新 | `UpdateKnowledgeTag`、`UpdateKnowledgeTagBatch` | 更新前 tag IDs/hash | 更新后 tag IDs/hash | `tag_update` | tag 未变化跳过版本 |
| `suggest_to_team` apply | `internal/wika/suggestion` apply 路径 | source suggestion、target knowledge baseline | apply 后团队 knowledge 快照 | `suggestion_apply` | suggestion 幂等键命中时不重复记录 |
| URL refresh apply | `internal/wika/governance/urlrefresh.Service.applyJob` | apply 前 knowledge baseline | fetched content 写入后的快照 | `url_refresh_apply` | job 非 `pending_review` 或已 applied 不记录 |
| freshness 处理 | `internal/wika/freshness` 处理动作 | 处理前状态/有效期/正文 hash | 处理后状态/有效期/正文 hash | `freshness_handle` | 只在影响标题、正文、标签、状态、有效期时记录 |
| restore | `internal/wika/governance/version.Service.Restore` | 当前 knowledge 快照 | restore 后快照 | `restore` | restore context 必须设置 `restored_from_version_id` 和 skip 标记，防止二次 baseline |

实现要求：P5b-4 先写 inventory 测试锁定上表入口；任何入口暂时无法接 hook 时，该入口对应 apply/restore/处理动作不能 GA。不要在多个 handler 手动散落 `RecordVersion`，应优先做 `KnowledgeMutationWithVersion` 或等价 wrapper。

必测：

- 连续更新形成递增版本。
- restore 不覆盖历史版本。
- A 不能读取 B personal knowledge 版本正文。
- SystemAdmin 查看 personal version list 只有 `version_no/content_hash/change_reason/created_at`，没有正文和 diff。
- `Diff` 对无权用户和 SystemAdmin personal 场景不得返回 diff text、old/new content、snippet，只能返回元数据或 404。
- `suggest_to_team` apply、URL apply、restore 三条写路径都形成版本。

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
- schedule 触发 job 必须写 `scheduled_for` 或幂等键；同一 `schedule_id + scheduled_for` 重复触发返回已有 job。
- `source_url` 默认来自知识已有来源；Contributor 不能任意覆盖 URL，Admin/Owner 覆盖 URL 时必须审计并重新经过 SSRF 全量校验。
- 连续失败默认第 1 次延后 1 小时、第 2 次延后 6 小时、第 3 次停用 schedule；失败码写入 `last_failure_code`。
- P5c worker contract：`RunDueSchedules(ctx, now)` 只创建或返回 due jobs；worker 再对返回 jobs 调 `RunJob(ctx, RunJobInput{JobID, WorkerID, Now, LeaseDuration})`。worker 不直接 fetch URL，也不直接写知识正文。
- `RunJob` 抓取失败时必须落 `failure_code` 和脱敏 error 摘要；如果关联 schedule 达到阈值，必须在同一状态推进中更新 schedule backoff/disable 并写 `wika.url_refresh.schedule_disabled` 或 `schedule_updated`。

必测 fixture：

- `http://127.0.0.1`、`http://[::1]`、`http://169.254.169.254`、metadata hostname/CNAME、云厂商特殊 metadata 地址、URL userinfo、percent-encoding、十进制/八进制 IP、内网域名、重定向到内网、DNS rebinding、超大响应、解压炸弹、非 HTML/文本类型全部被拒绝。
- 正常 URL 成功后状态为 `pending_review`，不会直接覆盖知识正文。
- `pending_review` 前调用 review/apply 返回 `WIKA_STATE_CONFLICT`。
- apply 后必须产生 `wika_knowledge_versions` 新版本和 `wika.url_refresh.reviewed` 审计事件。
- 抓取正文、HTML、diff 摘要中包含 prompt injection 时，仍只能进入 `pending_review`，不能自动 apply 或改变 allowed_fields。

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
- schedule 创建 run 必须带 `schedule_id + scheduled_for` 幂等键；同一 slot 重试不得重复创建 run。
- 连续失败默认第 1 次延后 1 小时、第 2 次延后 6 小时、第 3 次停用 schedule；失败码写入 `last_failure_code`。
- P5d GA 前必须补齐 run 创建与 schedule 推进的原子性证据。可接受两种实现：一是让 P2 EvaluationService/store 识别 `database.WithGormTransaction(ctx, tx)` 并在同一 tx 内创建 run/run_items/complete，再推进 schedule；二是新增 `RunScheduledEvaluationTx` 或等价 adapter，由 evalschedule service 在单个 tx 中调用 run 创建和 `MarkScheduleTriggered`。
- Alpha 阶段如果暂时保留“先创建 run，再推进 schedule”的现状，必须依赖 `eval_runs(schedule_id, scheduled_for)` 唯一约束证明重复触发不会产生重复 run，并在 P5d-5 中补上 tx-aware 实现或明确不可 GA。
- 审计失败时 schedule create/update/disable、失败 backoff/disable 都不得推进状态；worker 失败日志可以异步，但 failure counter 和 failure_code 必须落库。

必测：

- cron 解析错误返回 400。
- 连续失败达到阈值后状态变化和 audit log 存在。
- 禁用 schedule 不会触发新 run。
- 两个 scheduler 实例同时扫描 due schedule 时只创建一个 eval run。
- schedule 创建 run 时复用 P2 EvaluationService，不复制 metric 逻辑。
- worker crash 后重试同一 scheduled slot 不重复创建 run。

#### OrganizationShareService

职责：

- 读取或委托既有 OrganizationService 管理 Organization 和成员团队。
- 管理 P5 Wika 跨团队 KB 引用共享授权。
- 将 active share 暴露给 ScopeResolver。
- 撤销后阻止新搜索命中。

最小方法：

```text
CreateShare(ctx, actor, orgID, sourceKBID, targetTenantID, allowedFields)
AcceptShare(ctx, actor, shareID)
RevokeShare(ctx, actor, shareID)
ListSharedScopes(ctx, actor, tenantID)
```

可选 facade：

```text
CreateOrganization(ctx, actor, payload)            # 仅委托既有 OrganizationService
AddOrgMember(ctx, actor, orgID, tenantID, role)    # 仅委托既有 OrganizationService
```

实现规则：

- 复用现有 `organizations` 和 `organization_tenant_members`；P5e 只新增 Wika share 授权层，不新增平行 Organization 主表。
- 当前实现边界：组织创建、列表、成员管理可以先走既有 `/api/v1/organizations*`；P5e share service 只读取既有 org/member 关系并执行 source/target team Admin/Owner 数据授权。`/api/v1/wika/orgs*` facade 不得阻塞 P5e share GA。
- P5 默认 `mode=reference`，不复制正文。
- `allowed_fields` 默认只允许 ID、标题、来源团队、质量分、保鲜状态，不含正文、chunk、证据、文件。
- SearchService 命中 shared scope 后仍要用 ScopeResolver 输出的 `allowed_fields` 裁剪结果。
- direct knowledge read、expand、download、preview 使用 shared ID 时也必须重新走 ScopeResolver 和 `allowed_fields`，禁止通过二跳读取正文、chunk、证据、文件或 diff。
- revoke 后 ScopeResolver 不能再返回该 share；历史访问日志和 lineage metadata 保留。
- personal tenant 禁止加入 Organization，personal KB 禁止作为 Organization share source。
- `allowed_fields` 只能从服务端白名单枚举中选择，客户端传入未知字段返回 400。
- `CreateShare` 要求 source team Admin/Owner；当创建者同时具备 target team Admin/Owner 时可直接 active，否则进入 pending。org role 不能单独让 share active。
- `AcceptShare` 要求 target team Admin/Owner，成功后 active，才会进入 ScopeResolver。
- `RevokeShare` 允许 source team Admin/Owner 或 target team Admin/Owner 执行；org admin 只有同时具备 source/target team Admin/Owner 时可执行。

必测：

- 未加入接收团队的用户不能通过 org share 搜索。
- pending share 不进入 ScopeResolver。
- revoke 后同一 query 不再返回共享结果。
- revoke 后 direct read、expand、download、preview 同一 shared ID 都不再返回共享内容。
- SystemAdmin 不因 Organization 共享获得个人或团队正文读取权。
- personal tenant 禁止加入 org；personal KB 禁止作为 source。
- 客户端传入 `content`、`chunk`、`evidence_text`、`file` 等未知或禁用字段时返回 400。
- org admin 不是 target team Admin/Owner 时调用 `AcceptShare` 返回 403。

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
| 000101 | Wika org shares；复用既有 organizations / organization_tenant_members |
| 000102 | Wika org shares `allowed_fields` DB allowlist check；只约束 P5 新表 |

当前上游迁移已到 `000063`，`000090+` 仍留有缓冲。

P5 migration 必测约束：

- `wika_conflict_items` 同一 canonical `pair_key + conflict_type` 未终态唯一，必须覆盖 A->B 和 B->A 反向重复。
- `wika_knowledge_versions(knowledge_id, version_no)` 唯一。
- `wika_url_refresh_jobs(schedule_id, scheduled_for)` 在 `schedule_id IS NOT NULL` 时唯一。
- `wika_url_refresh_schedules(knowledge_id, source_url)` 启用状态唯一。
- `eval_runs(schedule_id, scheduled_for)` 或等价触发表唯一，防止同一 schedule slot 重复创建 run。
- `wika_eval_schedules(kb_id, dataset_id)` 启用状态唯一。
- `organization_tenant_members(organization_id, tenant_id)` 复用既有唯一约束；P5e 不新增平行成员表。
- `wika_org_shares` pending/active 状态下同一 `source_kb_id + target_tenant_id` 唯一。
- `wika_org_shares.allowed_fields` 必须有 DB allowlist CHECK，非法 `content`、`file`、`chunk`、`evidence_text` 不能落库；读取侧仍要二次 fail-closed。
- down migration 必须按 Wika org share、eval schedule、url refresh schedule/job、version、conflict 的依赖顺序回滚；不得 drop 或改写既有 `organizations`、`organization_tenant_members`、`kb_shares`。

P5 迁移落地状态：

- `000097`、`000098`、`000099` 已有实现时，进入对应 P5a/P5b/P5c 后续任务前必须先对齐本文档约束和测试。
- `000097` 必须补齐或确认：`failure_code`、`max_attempts`、`next_run_at`、canonical pair 唯一约束；worker runnable 索引不得把终态 `failed` 当作可领取状态。
- `000098` 的 `status/review_status` 是知识快照字段，可作为 DDL CHECK 例外；写入 hook 必须保证来自当前知识状态枚举或空值。
- `000099` 必须补齐或确认：`wika_url_refresh_jobs(schedule_id, scheduled_for)` slot 唯一、job 状态 check、schedule enabled 唯一、due schedule 索引、`schedule_id` 引用关系、失败计数和 down migration 不影响已生成知识版本。
- `000100` 已存在：继续 P5d 前先确认 `wika_eval_schedules` enabled 唯一、due schedule 索引、`eval_runs(schedule_id, scheduled_for)` slot 唯一和 down migration 均与本文档一致；不要重复新建迁移，缺口用追加迁移或修正测试驱动补齐。
- `000101` 已存在：继续 P5e 前先确认 `wika_org_shares` 状态 check、open share 唯一、target/source 查询索引和 down migration 均与本文档一致；不要新建平行 Organization 或成员迁移。
- `000102` 已存在：继续 P5e 前先确认 `allowed_fields` DB allowlist check、down migration 和读取侧二次过滤测试均与本文档一致。

P5 migration 建议索引：

| 表 | 索引/约束 | 用途 |
|----|-----------|------|
| `wika_conflict_checks` | `(tenant_id, kb_id, status, created_at DESC)` | 队列列表和最近 check |
| `wika_conflict_checks` | `(status, next_run_at, locked_until, created_at)` where status = `pending` | worker 领取 |
| `wika_conflict_items` | unique `(tenant_id, kb_id, pair_key, conflict_type)` where status in (`open`, `confirmed`) | 未终态去重，覆盖反向 pair |
| `wika_conflict_items` | `(tenant_id, kb_id, status, updated_at DESC)` | 冲突队列筛选 |
| `wika_knowledge_versions` | unique `(knowledge_id, version_no)` | 版本递增 |
| `wika_knowledge_versions` | `(tenant_id, kb_id, knowledge_id, version_no DESC)` | 版本列表 |
| `wika_url_refresh_jobs` | `(tenant_id, kb_id, knowledge_id, created_at DESC)` | 知识详情 job 列表 |
| `wika_url_refresh_jobs` | `(status, locked_until, created_at)` where status = `pending` | worker 领取 |
| `wika_url_refresh_jobs` | unique `(schedule_id, scheduled_for)` where schedule_id is not null | schedule slot 幂等 |
| `wika_url_refresh_schedules` | unique `(knowledge_id, source_url)` where enabled = true | 同源启用计划唯一 |
| `wika_url_refresh_schedules` | `(enabled, next_run_at, locked_until)` | due schedule 扫描 |
| `eval_runs` | unique `(schedule_id, scheduled_for)` where schedule_id is not null | eval schedule slot 幂等 |
| `wika_eval_schedules` | unique `(kb_id, dataset_id)` where enabled = true | 同 KB/数据集启用计划唯一 |
| `wika_eval_schedules` | `(enabled, next_run_at, locked_until)` | due schedule 扫描 |
| `organization_tenant_members` | existing unique `(organization_id, tenant_id)` | 复用既有成员唯一约束 |
| `organization_tenant_members` | existing `(tenant_id)`、`(organization_id, role)` | 查询团队所属 org |
| `wika_org_shares` | unique `(source_kb_id, target_tenant_id)` where status in (`pending`, `active`) | 待确认或 active share 唯一 |
| `wika_org_shares` | `(target_tenant_id, status)` | ScopeResolver 解析 shared scope |
| `wika_org_shares` | `(source_tenant_id, source_kb_id, status)` | 来源团队共享管理 |

DDL 规则：

- 所有状态字段必须用 check 约束或等价枚举校验，不能只靠应用层字符串。
- P5b `wika_knowledge_versions.status/review_status` 是上游知识状态快照字段，是 DDL CHECK 例外；service 写入时必须按当前知识状态来源复制，不接受客户端输入。
- 所有 JSON/JSONB 字段都必须有默认空对象或空数组，避免 handler 里处理 nil 分支。
- 外键字段都要有普通索引；涉及跨表删除时优先 restrict，不级联删除治理历史。
- down migration 只能删除 P5 新表和索引，不删除或改写 `knowledges` 历史数据。
- P5c down：停止 worker/schedule 后只删除 URL refresh 新表和索引；已 apply 的知识版本保留在 `wika_knowledge_versions`。
- P5d down：删除 `wika_eval_schedules` 和新增 schedule slot 索引/字段前，必须保留已创建的 `eval_runs/eval_run_items`；不能删除 P2 评测历史。
- P5e down：只删除 `wika_org_shares` 及 shared scope 相关索引/代码入口；不得删除既有 Organization、成员和 `kb_shares` 数据。
- 如果 PostgreSQL 迁移使用 `CREATE INDEX CONCURRENTLY`，必须拆出非事务迁移；否则本项目迁移框架不支持时先使用普通索引并在发布窗口执行。

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

## 十四、P5 生产回滚 Runbook

P5 回滚优先关闭能力，不删除历史数据。除非迁移本身导致启动失败，否则不要先跑 down migration。

1. **定位子阶段**：确认事故属于 `conflict`、`version`、`url_refresh`、`eval_schedule` 或 `org_share`。不要一次性关闭所有 P5，除非存在统一 scope 或审计基础设施事故。
2. **关闭 feature flag**：把对应 `wika.governance.*.enabled` 置为 false；确认 missing/store error/invalid value 也会 fail closed。
3. **停止 worker**：P5a/P5c/P5d 通过 ResourceCleaner 或进程重启触发 worker `Stop`；确认没有新的 lease、job 或 run 产生。
4. **确认停写语义**：写 API 返回 `WIKA_FEATURE_DISABLED` 或稳定 not found；已有治理记录只读可查。P5c `pending_review` 可 reject 但不可 apply；P5d enabled schedule 显示系统暂停且不创建 run；P5e shared scope 不再参与 search/expand/direct-id。
5. **检查 stuck lease**：查询 `locked_until > now` 的 job/schedule/check；只有确认对应 worker 已停止且 lease 超时策略失效时，才由维护者手动释放 lease，并记录审计或运维日志。
6. **验证回滚**：跑对应卡的 flag off 测试和最小 HTTP/MCP 冒烟；至少验证 search、expand、direct-id 或 worker due path 不再产生新状态。
7. **迁移回滚**：只有 DDL 阻塞启动、写入或查询时才执行 down migration。down 只能删除 P5 新表、索引和约束，不删除既有 `knowledges`、`organizations`、`organization_tenant_members`、`eval_runs` 和 audit 历史。
8. **恢复策略**：修复后先在 Internal Alpha 用 `RunOnce` 或最小 fixture 打开，再进入 Pilot；不得直接恢复 GA 放量。

## 十五、完成门禁

不能只靠代码存在判断完成。每期必须提供证据：

- 数据库迁移 up/down。
- Go 单元测试。
- API 冒烟。
- 涉及 MCP 的阶段必须优先提供 HTTP Remote MCP 真实调用；stdio 只可作为旧客户端兼容补充证据。P5 不新增 MCP 工具，只回归 `search_knowledge` shared scope。
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

P5 本地实现与验收流程：

1. 选择唯一任务卡；当前继续 P5 时以“P5 当前可实施任务队列”为准，不以历史 P0-P5 索引作为下一步排序。
2. 跑当前卡最小测试包，确认失败原因是生产代码缺能力，不是 fixture 或环境错误。
3. 最小实现后只扩大到当前卡相关 package；最后再跑本表对应阶段命令和 `git diff --check`。
4. 需要 API 冒烟时启动本地服务：

```bash
make dev-start
make migrate-up
make dev-app
make dev-frontend
```

5. 全量 `go test ./...` 不是 P5 单卡完成的唯一门禁；如果全量测试命中 docreader、外部存储、第三方 token 或历史包问题，必须记录失败包和错误摘要，不能声称全量通过。P5 单卡仍必须保证相关 package 通过。
6. 涉及 feature flag 的卡，必须在 flag 缺失、关闭、开启三种状态各跑一次关键路径；flag 关闭时写 API 返回 `WIKA_FEATURE_DISABLED` 或稳定 not found，worker 不领取 lease。
7. 涉及 worker 的卡，必须提供 `RunOnce` 或等价手动触发证据；只看到定时 goroutine 启动日志不能算完成。

P5 MCP 回归口径：

- P5 不新增 MCP tool；P5e 只回归已有 `search_knowledge` 和 `expand_knowledge_result`。
- MCP 真实调用必须使用用户级 PAT，通过 Remote MCP 请求头 `Authorization: Bearer <PAT>` 透传；管理工具另加 `X-Wika-MCP-Toolset: admin` 并使用 `mcp:admin` scope；不得用 `WEKNORA_API_KEY` 作为日常工具 fallback。
- P5e 冒烟最小路径：
  1. source team Admin 创建 active share，`allowed_fields=["id","title"]`。
  2. target team 成员通过 MCP `search_knowledge(query, include_team=true)` 命中 shared 结果。
  3. 返回结果标记 `source=shared`，不得包含正文、chunk、证据、文件、metadata。
  4. 同一成员调用 `expand_knowledge_result([id])`，仍只返回 allowed fields。
  5. source 或 target team Admin revoke share。
  6. 重复 `search_knowledge`、`expand_knowledge_result`、direct read/download/preview，同一 shared ID 不再返回共享内容。
- MCP 冒烟只验证消费路径；create/accept/revoke 仍以 HTTP API 或前端队列验收。

P5 每卡证据模板：

| 任务卡 | 测试命令 | fixture / API 冒烟 | 审计证据 |
|--------|----------|--------------------|----------|
| P5a-1/P5a-2/P5a-3 Conflict | `go test ./internal/types ./internal/wika/governance/conflict ./internal/handler ./internal/router ./internal/container -count=1` | migration up/down、canonical pair fixture；`POST /api/v1/wika/kb/:id/conflicts/checks`、`PUT /api/v1/wika/conflicts/:id`、flag off | `wika.conflict.check_created`、`wika.conflict.item_confirmed/dismissed/resolved` |
| P5a-4 Conflict gate/lifecycle | `go test ./internal/wika/governance/conflict ./internal/handler ./internal/container -count=1` | worker start/stop、flag off 不 lease、manual check API 冒烟、终态 item 不回 open | `wika.conflict.check_failed`、audit fail rollback |
| P5b-1/P5b-2/P5b-3 Version | `go test ./internal/types ./internal/wika/governance/version ./internal/handler ./internal/router ./internal/container -count=1` | migration up/down、baseline fixture；`GET /api/v1/wika/knowledge/:id/versions`、`POST /api/v1/wika/knowledge/:id/versions/:version_id/restore`、无权 diff | `wika.version.recorded`、`wika.version.restored` |
| P5b-4 Version hook inventory | `go test ./internal/wika/governance/version ./internal/handler -count=1`，并追加被接入写路径的 package | Web/旧 API/suggestion apply/URL apply/freshness/restore 每条路径都有 baseline 和新版本；hook 防递归 | 无版本记录的 apply/restore 不允许上线 |
| P5c-0/P5c-1/P5c-2/P5c-3/P5c-4 URL refresh | `go test ./internal/types ./internal/wika/governance/urlrefresh ./internal/handler ./internal/router ./internal/container -count=1` | migration up/down、SSRF fixture；`GET/POST /api/v1/wika/knowledge/:id/url-refresh`、`PUT/DELETE /api/v1/wika/knowledge/:id/url-refresh/schedules/:schedule_id`、`PUT /api/v1/wika/url-refresh/:refresh_id/review`、flag off | `wika.url_refresh.job_created`、`job_failed`、`reviewed`、`schedule_updated` |
| P5c-5 URL refresh worker/audit | `go test ./internal/wika/governance/urlrefresh ./internal/container -run 'TestWorker|URLRefresh' -count=1` | worker `Start/Stop/RunOnce`、flag off 不调用 due/job、连续失败后 schedule 延后或 disable、review/apply API 冒烟 | `wika.url_refresh.schedule_disabled` 或 `schedule_updated`；抓取正文和 URL 明文不进审计 |
| P5d-1/P5d-2/P5d-3 Eval schedule | `go test ./internal/types ./internal/wika/governance/evalschedule ./internal/handler ./internal/router ./internal/container -count=1` | migration up/down、cron fixture；`POST /api/v1/wika/kb/:id/eval/schedules`、schedule slot 幂等、disable/flag off 后 worker 不触发 | `wika.eval_schedule.updated`、`run_failed` |
| P5d-4 Eval worker lifecycle | `go test ./internal/wika/governance/evalschedule ./internal/container -run 'TestWorker|EvalScheduleWorker' -count=1` | worker start/stop、flag off 不调用 runner、ResourceCleaner 停止、重复 Start 幂等 | worker lifecycle 本身不写业务审计；必须有结构化日志和 disabled 指标 |
| P5d-5 Eval schedule audit/API smoke | `go test ./internal/wika/governance/evalschedule ./internal/handler ./internal/container -count=1` | `GET/POST/PUT/DELETE /api/v1/wika/kb/:id/eval/schedules` 200/400/403/409；due lag、failure disable 可查；P2 run 原子性证明 | `wika.eval_schedule.updated`、`wika.eval_schedule.run_failed`；audit fail rollback |
| P5e-1/P5e-2/P5e-3/P5e-4/P5e-5 Org share | `go test ./internal/types ./internal/wika/governance/orgshare ./internal/wika/scope ./internal/wika/search ./internal/handler ./internal/router ./internal/container -count=1` | migration up/down、share/accept/revoke fixture；MCP `search_knowledge` shared scope、download/preview/direct read 裁剪回归 | `wika.org_share.created`、`wika.org_share.accepted`、`wika.org_share.revoked` |
| P5e-6/P5e-7 Org share hardening | `go test ./internal/wika/governance/orgshare ./internal/wika/scope ./internal/wika/search ./internal/handler ./internal/router ./internal/container -count=1` | `POST /api/v1/wika/knowledge/expand` active shared 裁剪、revoke 后 search/expand/direct-id 不命中、MCP search/expand 真实调用 | create/accept/revoke 与状态转换同事务；审计失败不推进状态 |
| P5e-8 Org share real smoke/front-end | `go test ./internal/wika/governance/orgshare ./internal/wika/scope ./internal/wika/search ./internal/handler -count=1`，前端有改动时加前端测试 | 真实 source/target/non-member fixture；HTTP create/accept/revoke；MCP search/expand；revoke SLA <= 60s；必要时前端队列入口 | 双方审计可查；前端不拼权限，只消费后端状态 |
