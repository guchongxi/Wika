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
| ADR-04 | 日常 MCP 工具使用用户级 PAT/OAuth | 不能用租户 API key 伪装 Admin 写入个人知识 |
| ADR-05 | `search_knowledge` 使用 scope-aware fan-out hybrid search | 先解析用户可访问 scope，再复用现有 KB hybrid-search 能力 |
| ADR-06 | `suggest_to_team` 默认不自动发布到团队 | 默认进入可应用结果；只有团队策略开启且确定性安全门禁通过才自动应用 |
| ADR-07 | 旧 KB/knowledge/search/download/preview API 必须接入 personal scope | 只保护 Wika 新 API 会留下 IDOR |

## 四、总体架构

保持模块化单体，不拆微服务。

新增 Wika 编排层：

```text
internal/wika/
  space/          # Space 门面，封装 Tenant/tenant_members
  auth/           # 用户级 MCP PAT/OAuth scope
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
  expected_chunk_ids jsonb
  tags jsonb
  enabled
  created_at
  updated_at

eval_runs
  id
  tenant_id
  kb_id
  dataset_id
  trigger
  status
  mrr
  recall_at_5
  ndcg_at_5
  metrics jsonb
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
  retrieved_chunk_ids jsonb
  retrieved_knowledge_ids jsonb
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
  created_at
  updated_at
```

不要把扫描明细塞进一个大 JSON 数组。

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

新增 PAT/OAuth 能力：

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

### 7.2 日常工具

```text
push_knowledge(title, content, source?, tags?, evidence?, expires_at?, idempotency_key?, dry_run?)
search_knowledge(query, limit?, include_team?, format?)
expand_knowledge_result(ids[])
get_my_knowledge(limit?, status?, tag?)
suggest_to_team(knowledge_id, target_space_id?, reason?)
```

`dry_run=true` 时只返回规范化、质量分、疑似重复和建议，不入库。

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
  AdminKbDefaults.vue
```

改造现有页面：

- 顶部空间切换。
- KB 创建弹窗默认只显示名称、描述、类型。
- 高级配置折叠。
- 知识详情增加“推荐到团队”入口。
- 团队维护者首页突出 `待确认` 队列。

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

### P1 闭环 MVP

交付：

- `tenants.space_type` / personal space。
- `user_personal_spaces`。
- 默认 KB。
- 空间策略 `wika_space_policies`，团队自动应用默认关闭。
- 用户级 MCP token。
- `wika_knowledge_state` 最小字段。
- `knowledge_access_daily`。
- `push_knowledge`。
- `search_knowledge` compact。
- `expand_knowledge_result`。
- `get_my_knowledge`。
- `suggest_to_team` AI 预审和人工覆盖。
- 团队复制入库。
- 旧 KB/knowledge/search/download/preview API personal scope 改造。

验证：

- A/B 个人隔离。
- Web 和 MCP 都能写入。
- 个人 + 团队检索可用。
- `通过 / 待确认 / 不通过` 三态可构造。
- 默认不开启自动应用时，AI 通过不会自动进入团队。
- 开启自动应用且安全门禁通过时，AI 通过可自动复制到团队。
- 恶意知识、敏感内容、旧接口越权访问均被阻断。

建议实施顺序：

1. 迁移和类型：space 类型、personal space 表、默认 KB、空间策略、用户 token、knowledge state、access daily、suggestion/lineage。
2. SpaceService：注册/首次访问创建个人空间，空间列表和切换。
3. Auth：用户级 token 创建、撤销、scope 校验，MCP 日常工具拒绝租户 API key。
4. 旧 API scope 改造：KB/knowledge/search/download/preview/hybrid-search 先通过 A/B 越权测试。
5. Intake：Web/MCP 共用 `push_knowledge`，幂等、质量分、默认 KB。
6. Search：scope resolver、fan-out hybrid-search、compact/expand、access daily。
7. Suggestion：AI 修正、schema 校验、三态、人工覆盖、幂等 apply。
8. Frontend：空间切换、个人知识、推荐队列、团队自动应用策略入口。

### P2 质量闭环

交付：

- eval dataset。
- eval QA items。
- eval runs。
- eval run items。
- 趋势接口。
- 单条 dry-run。

验证：

- 5 条 QA 可评测。
- 指标和 case 明细可复查。

### P3 保鲜闭环

交付：

- freshness checks/items。
- 保鲜面板。
- 手动处理动作。
- 访问聚合驱动的长期未访问扫描。

验证：

- 过期、将过期、长期未访问、低质量均可构造和处理。

### P4 发现增强

交付：

- 图谱浏览。
- 图谱详情。
- 图谱增强检索权重调优。

### P5 高级治理

交付：

- 冲突检测。
- 版本 diff 和恢复。
- 自动 URL 重抓。
- 定时评测。
- Organization 跨团队引用共享。

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

当前上游迁移已到 `000063`，`000090+` 仍留有缓冲。

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

## 十四、完成门禁

不能只靠代码存在判断完成。每期必须提供证据：

- 数据库迁移 up/down。
- Go 单元测试。
- API 冒烟。
- MCP stdio 或 HTTP 真实调用。
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
