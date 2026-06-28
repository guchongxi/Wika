# Wika 改造需求方案

> 版本: 2.4
> 日期: 2026-06-29
> 基于: WeKnora v0.6.2 fork
> 状态: P0-P5 方案可拆分实施，P5 已补齐到可写 RED 测试和进入实现口径

## 一、目标

Wika 要解决的问题不是“再做一个知识库页面”，而是建立一套完整的知识闭环：

```text
知识生产 -> 评测 -> 存储 -> 消费(个人和团队) -> 保鲜 -> 再生产
```

核心目标：

- 任何人都能通过 AI 工具快速生产知识，首选接入方式暂定 MCP。
- 不使用 AI 工具的人，也能通过 Web 页面手动创建、管理和更新知识。
- 知识先进入个人空间，经过推荐、审核和修正后沉淀为团队资产。
- 团队成员和 AI 工具能在后续工作中自动复用这些知识。
- 系统能持续评测知识库质量，并发现过期、低可信、重复或需要确认的知识。

一句话定位：

> Wika 是面向个人与团队的 AI 原生知识闭环系统，让人和 AI 工具都能低摩擦地产生、评测、沉淀、复用和保鲜知识。

北极星指标：

> 每周有效知识复用次数。

有效复用包括：AI 工具检索后引用、用户打开并使用、个人知识被推送到团队、团队知识命中评测用例、保鲜处理后继续被检索。

## 二、产品原则

1. **目的优先**：所有功能围绕知识闭环服务，不为了“看起来完整”堆功能。
2. **个人先沉淀，团队再收益**：默认写入个人空间，团队空间不接受未经治理的自动灌入。
3. **AI 辅助，不替代治理**：AI 可以修正、评分、预审，但人工可以覆盖所有结论。
4. **低摩擦接入**：MCP 日常工具只暴露简单语义，不要求用户理解 `kb_id`、模型、分块策略等技术参数。
5. **可验证**：每个阶段必须有明确验收标准，不能只新增页面或注册工具就声明完成。
6. **上游友好**：尽量复用 WeKnora 的 Tenant、RBAC、知识管道、检索和评测基础能力，避免平行重造。

## 三、目标用户

| 用户 | 主要场景 | 价值 |
|------|----------|------|
| 开发者 | 在 Claude Code、Cursor 等工具里排查、复盘、搜索历史经验 | AI 自动读写知识，减少重复排查 |
| 普通 AI 用户 | 用 Web 页面或 AI 工具沉淀业务经验 | 不需要理解知识库技术细节 |
| 团队维护者 | 审核个人知识、维护团队知识质量 | 把个人经验转成团队资产 |
| 系统管理员 | 配置默认知识库参数、注册策略、空间概览 | 降低普通用户使用门槛 |

## 四、阶段范围

Wika 分两层验收：

- **基础闭环 P1**：证明个人生产、个人/团队检索、推荐团队、AI 预审、人工覆盖和安全隔离成立。
- **完整 MVP P3**：在 P1 基础上补齐评测和保鲜，才算完成“生产 -> 评测 -> 存储 -> 消费 -> 保鲜”的完整闭环。

完整 MVP 验证路径：

```text
个人生产 -> 个人检索 -> 推荐团队 -> AI 预审 -> 人工处理灰区 -> 团队检索 -> 评测 -> 保鲜
```

### P1 基础闭环必做

- 个人空间和团队空间显式化。
- Web 手动创建知识。
- MCP 日常工具：`push_knowledge`、`search_knowledge`、`get_my_knowledge`、`suggest_to_team`。
- `search_knowledge` 支持个人 + 团队的权限感知混合检索。
- `suggest_to_team` 支持 AI 自动修正和预审，输出 `通过 / 待确认 / 不通过`。
- 团队维护者主要处理 `待确认`，但可手动调整所有预审结果。
- 团队自动应用策略默认关闭；只有团队维护者显式开启并满足确定性安全门禁时，AI 判定 `通过` 的内容才可自动复制到团队。
- 系统级知识库默认配置，普通用户创建 KB 默认只填名称、描述、类型。

### P2-P3 完整 MVP 必做

- 黄金 QA 数据集、手动评测、评测 run 持久化、核心指标趋势。
- 基础保鲜：过期、低置信、长期未复用、手动更新/废弃。

### P4-P5 增强能力

- P4：图谱浏览、实体详情、图谱增强检索权重调优。
- P5：冲突检测、版本 diff 和恢复、自动 URL 重抓、定时评测、Organization 跨团队引用共享。

### 非目标

- 不做完整企业 Wiki 替代品。
- 不允许 AI 在未经过结构化预审、确定性安全门禁和团队策略授权时直接发布到团队空间。
- 不做复杂审批流引擎。
- P1-P3 不做自动 URL 重抓的完整数据源同步。
- P1-P3 不做 LLM 冲突检测和版本 diff 恢复。
- P1-P3 不把图谱可视化放进主路径。
- 不一次性迁移所有 `.env` 配置到系统后台。

### 阶段优先用户

| 阶段 | Primary 用户 | Secondary 用户 | 验证重点 |
|------|--------------|----------------|----------|
| P1 | 开发者、团队维护者 | 系统管理员 | AI 工具能安全写入/检索，团队维护者能治理个人到团队的流转 |
| P2 | 团队维护者 | 开发者 | 知识调整是否提升检索质量可被评测和追溯 |
| P3 | 团队维护者 | 普通 AI 用户 | 过期、低质、长期未复用知识能被发现和处理 |
| P4 | 普通 AI 用户、团队维护者 | 开发者 | 用户能通过图谱理解知识关系，检索能用图谱增强但不依赖图谱 |
| P5 | 团队维护者、系统管理员 | 开发者 | 高级治理、跨团队共享和自动化任务可受控运行 |

### 阶段实施故事与验证数据

后续开发按阶段验收，不按“页面是否存在”验收。每期必须先准备下表中的最小验证数据，再写测试和代码。

| 阶段 | 用户故事 | 最小验证数据 | 完成判定 |
|------|----------|--------------|----------|
| P0 | 作为开发者，我要知道哪些架构决策不可反向实现 | ADR-01 到 ADR-10、旧 API 清单、迁移编号规划 | 文档能直接回答 Space、PAT、scope、suggestion、评测、保鲜的实现边界 |
| P1a | 作为用户 A，我注册后自动有个人空间，且用户 B 无法通过新旧 API 读到 A 的个人知识 | 用户 A/B、A personal tenant、B personal tenant、团队 T、A/B 各一条个人知识 | 新旧 KB/knowledge/search/download/preview 路径对越权访问返回 404 或空结果 |
| P1b | 作为开发者，我能用 Web 或 MCP 写入个人知识，并用 AI 工具搜到个人和团队知识 | A 的个人知识 2 条、团队 T 知识 2 条、幂等键 1 个、无效 token 1 个 | `push_knowledge`、`search_knowledge`、`expand_knowledge_result`、`get_my_knowledge` 可真实调用并留下访问聚合 |
| P1c | 作为团队维护者，我主要处理 AI 无法确定的知识，必要时可覆盖所有 AI 结论 | 高质量知识、敏感知识、重复知识、过期知识、prompt injection 样式知识各 1 条 | `suggest_to_team` 可构造通过、待确认、不通过；默认不自动发布；开启自动应用也必须过安全门禁 |
| P2 | 作为团队维护者，我能用黄金 QA 判断知识调整是否提升检索质量 | 至少 5 条 QA，其中 3 条有 expected IDs、1 条无 expected IDs、1 条越权引用 | 正式 run 有指标和 case 明细；无 expected IDs 只能 dry-run；导出不泄露无权正文 |
| P3 | 作为维护者，我能发现和处理过期、低质、长期未复用知识 | 过期、将过期、低质量、低置信、90 天无访问知识各 1 条 | scanner 可生成保鲜项；处理动作有审计；检索热路径不写 `knowledges` 主表 |
| P4 | 作为团队成员，我能通过图谱理解知识关系，且图谱失败不影响搜索 | 团队实体 3 个、关系 3 条、个人实体 1 个、模拟图谱服务失败 | 团队图谱可分页；个人图谱受 Owner 限制；搜索返回图谱贡献或降级标识 |
| P5 | 作为团队维护者，我能处理冲突、恢复版本、确认 URL 更新、运行定时评测、授权跨团队引用 | 冲突候选、两个知识版本、URL 重抓 fixture、评测计划、Organization share/accept/revoke 各 1 组 | 每个高级治理动作都有状态流转、回滚或撤销路径、scope 校验和审计记录 |

### 阶段发布矩阵

阶段完成不是“代码合并”或“页面出现”，而是 Alpha 受控可用、GA 可放大、失败可回滚。下表是每期进入实现前必须写进测试和发布检查清单的产品口径。

| 阶段 | 非目标 | 成功指标目标值 | Alpha 门禁 | GA 门禁 | 回滚触发 | 回滚动作 |
|------|--------|----------------|------------|---------|----------|----------|
| P0 | 不写业务代码 | ADR-01 到 ADR-10 全部签准；旧 API inventory 100% 有 owner | 方案经产品、架构、安全确认 | P1a RED 测试清单可直接创建 | ADR 互相冲突或旧 API 清单缺失 | 停止进入 P1a，回到方案修订 |
| P1a | 不开放 AI 入库和团队推荐 | A/B IDOR 0 漏洞；旧 API 越权 0 漏洞；token 创建/撤销成功率 100% | 本地 A/B fixture 通过；租户 API key 调日常工具被拒绝 | 新旧 API scope 回归全绿；个人空间幂等创建通过 | 任一旧接口可读他人 personal 内容 | 关闭 Wika 日常入口，保留数据，修 scope |
| P1b | 不自动推荐团队；不做图谱增强 | MCP push/search 成功率 >= 99%；search P95 <= 1500ms；push 后 60s 内可检索 | Web 和 MCP 各完成 1 条真实入库/检索 | compact/expand/access 记录全链路通过 | 检索泄露无权 snippet 或入库重复失控 | 关闭 MCP 日常工具或只读 search |
| P1c | 默认不自动应用团队知识 | 三态 fixture 100% 可构造；自动应用误放行 0；人工覆盖成功率 100% | `approved/needs_confirmation/rejected` fixture 通过 | 默认关闭自动应用；开启后安全门禁全绿 | 敏感/prompt injection 内容自动进入团队 | 关闭自动应用策略，suggestion 保持人工队列 |
| P2 | 不用小样本指标代表整体质量 | 正式 run 100% 记录 case 明细；无 expected IDs 0 次进入正式指标 | 5 条 smoke QA 跑通导入、dry-run、正式 run | pilot dataset >= 30 条或团队确认样本代表性；趋势可追溯 | 越权导出 expected answer 或指标不可复现 | 停止正式 run 写趋势，只保留 dry-run |
| P3 | 不做自动 URL 重抓；不自动删除知识 | scanner 命中 5 类 fixture；处理动作 100% 有审计；热路径 0 主表写 | 单 KB 手动扫描通过 | worker 可停用；误报处理路径可用 | scanner 写放大影响搜索或误报率不可接受 | 停用 scanner/worker，保留 items 待人工清理 |
| P4 | 图谱不替代主搜索；图谱失败不阻断搜索 | 图谱增强开启后 search P95 退化 <= 20%；图谱故障主搜索成功率 100% | 实体详情和搜索降级 fixture 通过 | 图谱贡献率可观测；一键关闭增强有效 | 图谱泄露个人证据或导致搜索 500 | 关闭图谱增强开关，保留图谱读模型 |
| P5a Conflict | 不自动删除、覆盖、合并知识 | 冲突候选确认/驳回/解决全链路通过；同一未终态候选 0 重复 | 1 组冲突 fixture 通过 | 队列筛选、审计和人工处理可用 | AI 建议直接改变知识正文 | 关闭冲突检测 worker，只保留已生成候选 |
| P5b Version | 不覆盖历史版本 | restore 100% 生成新版本；版本正文越权 0 泄露 | 两个版本 diff/restore 通过 | 所有知识写路径均记录版本 | restore 覆盖历史或绕过 scope | 禁用 restore/API apply，版本列表只读；版本记录 hook 默认继续运行，除非 hook 自身导致写失败 |
| P5c URL Refresh | 不直接覆盖正文；不抓取非 HTTP(S) | SSRF fixture 100% 阻断；成功抓取 100% 进入待确认 | safe fetcher fixture 全绿 | 定时/手动 job 可停用，review/apply 有审计 | 内网/metadata/重定向绕过或恶意 HTML 执行 | 停用 URL refresh worker 和 schedule；已有 `pending_review` 只读保留，可 reject，不可 apply |
| P5d Eval Schedule | 不复制评测逻辑 | due schedule 触发成功率 >= 99%；连续失败后 100% 降频或停用 | 单 schedule cron 触发 run | 多实例锁验证通过；失败告警可查 | 重复触发 run 或无限失败重试 | 停用 schedule worker，保留手动评测；已有 enabled schedule 显示为系统暂停，不自动创建 run |
| P5e Org Share | 不共享个人正文、证据、文件；默认不复制 | revoke 后新 search 0 命中；allowed fields 裁剪 100% 生效；撤销传播 <= 60s | share/accept/revoke fixture 通过 | 接收团队权限和 shared scope 回归全绿 | 撤销后仍命中或字段越权 | 停用 shared scope，保留审计和 lineage 元数据；旧 shared ID 再 expand/direct read 返回无结果 |

## 五、功能需求

### FR-01 空间模型

Wika 将 WeKnora 的 Tenant 产品化为“空间”。

需求：

- Tenant 在 UI 中统一称为“空间”。
- 空间分为 `personal` 和 `team`。
- 用户注册后自动拥有一个个人空间。
- 个人空间只允许本人作为 Owner。
- 用户可以创建团队空间，并邀请成员加入。
- 团队空间复用现有 `tenant_members` 和 RBAC。
- 顶部导航可以切换当前空间。
- 切换空间后，知识库、Agent、设置等都以当前空间为上下文。

约束：

- 个人/团队边界是空间属性，不是知识库可见性字段。
- 不通过 `knowledge_bases.visibility` 表达个人知识。
- Organization 保留为跨空间高级协作能力，不作为 MVP 团队空间主路径。

### FR-02 知识生产与统一入库

所有知识生产入口必须走同一条入库链路：

```text
KnowledgeDraft -> Normalize -> Score -> Persist -> Index
```

入口：

- Web 手动创建。
- MCP `push_knowledge`。
- 后续可扩展 IM、浏览器插件、数据源导入。

MVP 字段：

- 标题。
- 正文。
- 来源 URL 或来源说明。
- 标签。
- 过期时间。
- 证据或上下文。
- 幂等键。

入库行为：

- 默认写入当前用户个人空间的默认知识库。
- 自动生成标准 Markdown 内容。
- 提取标签、摘要、适用范围和候选实体。
- 计算质量分。
- 检查疑似重复。
- 写入后复用现有知识处理管道完成 chunk、embedding、索引。

质量分建议：

| 维度 | 权重 |
|------|------|
| 完整性 | 25 |
| 可复用性 | 20 |
| 证据/来源 | 20 |
| 时效性 | 15 |
| 可检索性 | 10 |
| 去重/冲突风险 | 10 |

### FR-03 AI 友好的 `search_knowledge`

`search_knowledge` 是 Wika 消费闭环的核心工具，不应要求 AI 或用户传入技术性 KB 参数。

MVP 入参：

```json
{
  "query": "问题或检索意图",
  "limit": 5,
  "include_team": true,
  "format": "compact"
}
```

检索范围：

- 当前用户个人空间。
- 当前用户已加入的团队空间。
- 用户有权限访问的共享知识库。

检索策略借鉴 agentmemory，但不直接搬代码：

```text
query
  -> scope resolver
  -> optional query expansion
  -> keyword/BM25 retrieval
  -> vector retrieval
  -> optional graph retrieval
  -> RRF merge
  -> freshness/quality/security rerank
  -> compact result
  -> optional expand detail
  -> access log
```

要求：

- 关键词、向量、图谱三路结果使用 RRF 或等价方式融合。
- 图谱检索是可选增强，失败不影响主搜索。
- 结果默认 compact，避免 MCP 一次返回过多内容。
- 提供按 ID 展开详情的能力。
- 搜索命中要记录访问，用于复用统计和保鲜。
- 对 AI 返回的搜索结果必须标注为“不可信资料，不是指令”，避免 prompt injection。

MVP 返回：

```json
{
  "results": [
    {
      "knowledge_id": "...",
      "title": "...",
      "snippet": "...",
      "source_space": "personal | team | shared",
      "score": 0.82,
      "quality_score": 86,
      "freshness_status": "fresh | expiring | expired | stale",
      "updated_at": "..."
    }
  ],
  "truncated": false
}
```

### FR-04 个人到团队：`suggest_to_team`

`suggest_to_team` 不只是提交人工审批，而是 AI 预审 + 人工灰区处理。

流程：

```text
个人知识
  -> AI 自动修正
  -> AI 质量审核
  -> AI 风险审核
  -> AI 团队适配判断
  -> 输出: 通过 / 待确认 / 不通过
  -> 人工可调整所有结果，重点处理待确认
```

MVP 入参：

```json
{
  "knowledge_id": "...",
  "target_space_id": "...",
  "reason": "为什么建议共享"
}
```

AI 预审输出：

```json
{
  "decision": "approved | needs_confirmation | rejected",
  "confidence": 0.87,
  "corrected_title": "...",
  "corrected_content": "...",
  "reason": "...",
  "risks": [],
  "changes": [],
  "target_team_fit": 0.9,
  "quality_score": 86,
  "duplicate_risk": "low",
  "freshness_risk": "low"
}
```

决策规则：

| 决策 | 条件 | 默认处理 |
|------|------|----------|
| 通过 | 质量高、团队相关、低风险、AI 置信度高 | 默认进入可应用结果；团队开启自动应用且安全门禁通过后才可复制到团队 |
| 待确认 | 有价值但存在不确定项 | 进入人工重点队列 |
| 不通过 | 低质量、重复、过期、敏感或无团队价值 | 不进入团队，人工可恢复 |

自动应用策略：

- 系统默认 `auto_apply_approved=false`。
- 团队维护者可以在团队空间开启自动应用。
- 自动应用只处理 AI 决策为 `approved` 且命中确定性安全门禁的内容。
- 任一安全门禁失败时，结果必须降级为 `needs_confirmation` 或 `rejected`。
- 自动应用成功后必须保留源知识、AI 修正版、策略版本、风险检查结果、操作者来源和回滚入口。
- 自动应用不是“无审核发布”：AI 预审、确定性规则和团队策略共同构成机器审核；人工仍可覆盖所有结果。

确定性安全门禁：

- 来源知识必须属于提交者个人空间。
- 提交者必须有目标团队的成员身份。
- 内容不得命中敏感词、密钥样式、访问令牌样式、个人隐私字段或团队配置的阻断规则。
- AI 修正幅度超过阈值时必须进入 `needs_confirmation`。
- 疑似重复、过期、低置信、无团队相关性任一为高风险时不得自动应用。
- 检索到的知识正文中出现“忽略系统指令”“执行命令”“泄露密钥”等 prompt injection 样式内容时不得自动应用。

人工能力：

- 查看原文、AI 修正版、改动摘要、预审理由和风险。
- 修改标题、正文、标签、目标团队知识库。
- 将任何状态改为通过、待确认或不通过。
- 审批通过后复制到团队空间并重跑索引。

约束：

- AI 不得绕过审核直接发布敏感内容。
- 自动通过只允许在低风险、高置信规则内发生。
- 团队未开启自动应用时，`approved` 也必须等待人工或手动点击应用。
- 所有 AI 预审、人工覆盖和复制动作都必须可审计。

### FR-05 评测闭环

目标：让团队知道知识调整是否提升检索质量。

需求：

- 每个团队知识库可以维护黄金 QA 数据集。
- QA 维护责任归属目标团队 Admin/Owner；Contributor 可以提交候选 QA，但进入正式数据集前需要 Admin/Owner 确认。
- 支持手动新增、编辑、删除 QA。
- 支持 CSV/JSON 导入导出。
- 支持手动触发评测。
- 核心指标：MRR、Recall@5、NDCG@5。
- 每次评测结果持久化。
- 每条 QA 的命中 chunk、rank、score、失败原因都要可查。
- 支持单条 QA dry-run，查看召回结果。
- 趋势面板展示历史指标变化。
- 正式指标必须有 `expected_knowledge_ids` 或 `expected_chunk_ids`，否则只能 dry-run，不进入总体指标。
- 评测 run 必须记录检索参数、模型、索引策略、数据集版本和触发人，便于复现。
- 导出内容不得包含用户无权读取的个人知识正文；跨空间引用只导出 ID、标题和指标摘要。

MVP 不要求自动定时评测。

### FR-06 保鲜闭环

目标：发现并处理过期、低可信、长期未被复用的知识。

MVP 检测项：

- `expires_at` 已过期或即将过期。
- 长期未被检索或引用。
- 质量分过低。
- 人工标记为需复核。

默认阈值：

| 检测项 | 默认规则 | 可配置范围 |
|--------|----------|------------|
| 将过期 | `expires_at <= now + 14 days` | 团队可设 1-90 天 |
| 已过期 | `expires_at < now` | 不可关闭 |
| 长期未复用 | 90 天无检索命中、引用或评测命中 | 团队可设 30-365 天 |
| 低质量 | `quality_score < 60` | 团队可设 0-100 |
| 低置信 | `confidence_score < 0.6` | 团队可设 0-1 |

处理动作：

- 标记已更新。
- 延长有效期。
- 标记废弃。
- 重新提交团队审核。
- 查看最近访问与评测命中情况。

约束：

- 检索热路径不能每次直接更新知识主表。
- 访问记录应写入轻量事件或日聚合，再异步汇总。
- 保鲜扫描失败不能影响检索和知识读取。
- 处理动作必须记录操作者、处理前后状态和备注。

### FR-07 系统默认配置与管理

目标：普通用户不需要理解模型、分块、索引等技术配置。

需求：

- 系统管理员可以配置知识库默认值：
  - 默认 LLM 模型。
  - 默认 Embedding 模型。
  - 默认分块策略。
  - 默认索引策略。
  - 默认 VLM/ASR/问题生成开关。
  - 默认存储引擎。
- 普通用户创建知识库时默认只填名称、描述、类型。
- 高级配置默认收起。
- 默认值优先级：

```text
system_settings > 环境变量 > 硬编码 fallback
```

系统管理面板 MVP 只做必要能力：

- 用户概览。
- 空间概览。
- 知识库概览。
- 注册邮箱后缀白名单。
- 知识库默认配置。

完整 `.env` 可视化迁移后置。

### FR-08 图谱发现与增强检索

目标：让团队通过实体和关系理解知识，而不是只看文档列表。

P4 需求：

- 展示团队空间内的实体、关系、来源知识和最近更新时间。
- 支持实体详情页：实体摘要、相关知识、相关实体、关系证据。
- 支持从知识详情跳转到相关实体。
- 支持图谱增强检索：实体命中结果作为可解释的补充召回，不能替代权限过滤。
- 图谱检索失败时主搜索仍可返回 BM25/向量结果，并展示降级状态。

约束：

- 图谱节点和关系必须带 `tenant_id`、`source_knowledge_id` 和证据来源。
- 个人空间图谱只允许 Owner 查看；团队图谱按团队成员权限查看。
- SystemAdmin 只能看实体数量、关系数量、抽取任务状态等统计元数据。

### FR-09 高级治理

P5 覆盖高治理成本能力，只在 P1-P4 稳定后进入。

P5 按 5 个独立子阶段实施和发布：

| 子阶段 | 能力 | 稳定锚点 | 进入条件 |
|--------|------|----------|----------|
| P5a | 冲突检测 | `WIKA-P5-CONFLICT` | P1b search 和 P3 freshness state 可用 |
| P5b | 版本 diff 和恢复 | `WIKA-P5-VERSION` | 关键知识写路径已统一到可插 hook 的 service |
| P5c | 自动 URL 重抓 | `WIKA-P5-URL-REFRESH` | P5b version 已可记录 URL apply 后的新版本 |
| P5d | 定时评测 | `WIKA-P5-EVAL-SCHEDULE` | P2 evaluation run 已稳定可复用 |
| P5e | Organization 跨团队共享 | `WIKA-P5-ORG-SHARE` | P1a ScopeResolver 和 P1b Search 可扩展且基础回归通过；`shared scope` 由 P5e 实现并回归 |

P5 总体非目标：

- 不让 AI 自动修改、删除、覆盖或合并知识。
- 不让 URL 重抓直接覆盖正文。
- 不让 Organization 共享个人正文、证据、chunk 或文件内容。
- 不在 P5 内重写 P1-P4 的入库、搜索、评测和保鲜主链路。

#### 冲突检测

- 用户创建、更新或推荐知识时，系统可以生成疑似冲突候选。
- 冲突候选必须展示冲突类型、相似片段、证据知识和置信度。
- AI 可以给出解释和建议，但不能自动删除、覆盖或合并知识。
- P5a V1 默认只开放手动 check；`knowledge_updated`、`suggestion_applied` 等自动 trigger 先写入任务和测试契约，但 feature flag 关闭时不得自动排队。
- 冲突类型最小规则：`duplicate` 依赖内容 hash 或高相似度；`outdated` 依赖来源时间或保鲜状态；`scope_overlap` 依赖同主题覆盖范围重叠；`contradiction` 只能作为 AI/检索辅助候选，必须人工确认。
- 人工状态包含：`confirmed` 表示已确认存在冲突但尚未处理，`dismissed` 表示非冲突，`resolved` 表示已处理完成；终态只有 `dismissed` 和 `resolved`。
- 高相似但非冲突 fixture 必须进入 `open` 后可被人工标记为 `dismissed`，不能被 AI 自动确认或解决。

#### 版本 diff 和恢复

- 知识正文、标题、标签、状态等关键字段变更必须形成版本。
- 用户可以查看相邻版本 diff。
- 有权限的维护者可以恢复旧版本；恢复本身生成新版本，不能覆盖历史。
- 恢复个人敏感内容时仍按当前资源权限校验，SystemAdmin 不获得正文读取权。
- P5b 不要求 migration 一次性重写全部历史知识；既有知识首次发生版本化写入前必须生成 baseline 版本，再记录本次变更版本。

#### 自动 URL 重抓

- 对带来源 URL 的知识，团队可启用定时重抓。
- 重抓前必须通过 SSRF 防护：禁止内网地址、云元数据地址、非 HTTP(S)、重定向到禁用地址、超大响应和非允许内容类型。
- 重抓结果默认生成待确认更新，不直接覆盖知识正文。
- 用户可查看新旧内容 diff、来源响应摘要和抓取错误。
- 默认最小调度间隔为 1 小时；连续失败第 1 次延后 1 小时，第 2 次延后 6 小时，第 3 次停用 schedule，团队可配置但不能低于系统安全下限。
- 手动重抓必须限流；重复触发同一知识和同一来源 URL 时返回已有未终态 job 或稳定错误，不能刷出多个待确认更新。

#### 定时评测

- 团队可配置评测计划，按数据集和 KB 定时运行。
- 计划必须有启停、最近运行、失败原因和下一次运行时间。
- 连续失败要停止或降频，并提示团队维护者。
- 默认最小调度间隔为 1 小时；连续失败第 1 次延后 1 小时，第 2 次延后 6 小时，第 3 次停用 schedule，并记录失败码。
- 定时评测只创建 P2 evaluation run，不复制评测指标逻辑；同一 schedule 的同一触发时间最多创建一个 run。

#### Organization 跨团队共享

- Organization 只做 P5 跨团队引用共享，不替代 P1 团队空间。
- 共享方式优先是引用，不默认复制正文。
- 授权模型必须显式包含共享发起方、接收方、共享范围、可读字段和撤销规则。
- 撤销共享后，新检索不能再命中；历史审计保留元数据。
- P5e 复用现有 `organizations` 和 `organization_tenant_members` 作为组织和成员来源，不再新建平行 Organization 主表。
- 数据授权源头始终是团队 Admin/Owner：source team Admin/Owner 创建共享，target team Admin/Owner 接收共享；Organization admin 只能管理组织关系，不能替代团队授权扩大读取权限。

P5 默认安全边界：

- 冲突检测只能生成候选和建议，不自动删除、覆盖或合并知识。
- 版本恢复永远生成新版本，不覆盖历史版本。
- URL 重抓只生成待确认更新，不直接覆盖正文。
- 定时评测连续失败后必须停用或降频，不能无限重试。
- Organization 共享默认 `reference`，默认不包含个人正文、证据正文和文件内容。

#### P5 子阶段用户故事与验收口径

P5 不是一个大功能包，必须按 P5a-P5e 独立打开、独立回滚、独立验收。每个子阶段都只能在自身稳定锚点和功能开关开启后对用户可见。

| 子阶段 | 用户故事 | 主流程 | 终态 | 必须验收 |
|--------|----------|--------|------|----------|
| P5a Conflict | 作为团队维护者，我要看到疑似冲突知识并人工判断，避免团队知识互相矛盾 | 创建 check -> worker 生成 item -> 查看证据摘要 -> 确认冲突或驳回 -> 处理后标记解决 | `dismissed`、`resolved`；`confirmed` 是处理中间态 | AI 只给解释；任何状态流转都不改知识正文；同一未终态冲突不重复出现 |
| P5b Version | 作为团队维护者，我要知道知识被谁改过，并能恢复旧版本 | 写路径记录版本 -> 查看版本列表 -> 查看 diff -> restore | 新 `version_no` | restore 必须调用现有知识更新和索引链路，并生成新版本；历史版本只读 |
| P5c URL Refresh | 作为团队维护者，我要安全检查来源 URL 是否更新，再决定是否应用 | 创建 job 或 schedule -> safe fetch -> diff -> 人工 apply/reject | `applied`、`rejected`、`failed` | SSRF fixture 全部阻断；抓取成功默认 `pending_review`；apply 生成版本 |
| P5d Eval Schedule | 作为团队维护者，我要让稳定数据集定时评测，但失败时不能无限重试 | 创建 schedule -> worker 领取 due schedule -> 创建 run -> 更新 next_run 或 failure | `enabled=false` 或降频后的 enabled | 多实例不重复触发；失败达到阈值后停用或延后；run/case 明细复用 P2 |
| P5e Org Share | 作为团队维护者，我要授权其他团队引用本团队知识，同时可随时撤销 | 复用现有 org -> 加入团队 -> source team 创建 share -> target team accept -> shared scope 搜索 -> revoke | `revoked` | 默认 reference，不复制正文；pending 不进入搜索；allowed_fields 服务端白名单裁剪；revoke 后新搜索不命中；org admin 不能替代团队 Admin/Owner 扩大读取权限 |

P5 产品价值与成功指标：

| 子阶段 | 用户可观察动作 | 产品成功指标 | 事件/埋点 |
|--------|----------------|--------------|-----------|
| P5a Conflict | 维护者打开冲突队列，确认、驳回或标记解决候选 | open 队列积压 < 50；候选处理 P50 < 2 天；误报 dismiss 率可按类型查看；同一未终态 pair 0 重复 | `conflict_check_created`、`conflict_item_status_changed` |
| P5b Version | 维护者从知识详情查看版本历史、diff，并恢复旧版本 | 必须版本化写路径覆盖率 100%；restore 后搜索索引更新成功率 100%；无权 diff 泄露 0 | `version_list_opened`、`version_diff_opened`、`version_restored` |
| P5c URL Refresh | 维护者查看待确认 URL 更新，选择 apply 或 reject | 成功抓取进入待确认 100%；待确认处理率可查；apply/reject 平均处理时长可查；失败 schedule 占比可查 | `url_refresh_job_created`、`url_refresh_reviewed`、`url_refresh_schedule_disabled` |
| P5d Eval Schedule | 维护者启用固定数据集定时评测，查看失败原因和下一次运行 | due run 按时触发成功率 >= 99%；连续失败通知可查；低分趋势被处理数可追踪 | `eval_schedule_saved`、`eval_schedule_run_created`、`eval_schedule_failed` |
| P5e Org Share | source team 发起 share，target team accept 后搜索命中，任一方 revoke 后不再命中 | active shared 搜索命中字段裁剪 100%；revoke 后 search/expand/direct-id 0 命中；双方审计可查 | `org_share_created`、`org_share_accepted`、`org_share_revoked` |

P5 灰度和回滚口径：

- 每个子阶段都有独立 feature flag，默认关闭。
- feature flag 缺失、读取失败、非法值或缓存超过 TTL 时必须按关闭处理。
- 关闭 feature flag 后，新 API 写入和 worker 领取停止；已生成记录保持只读或可安全查看。
- P5 worker 必须可独立停用，不能影响 P1-P4 的入库、搜索、评测和保鲜。
- P5 API 不允许先暴露空实现；未开启时返回稳定错误，不能误导前端进入半可用状态。
- P5 前端只展示当前开启的子阶段入口；隐藏入口不是权限控制，后端仍必须执行 scope 和状态机校验。
- 关闭 worker 型能力后，UI/API 必须展示系统暂停或能力关闭状态；不能把未运行误导为正常 enabled。
- 关闭共享读取后，历史 shared ID 只能用于审计和 lineage 元数据，不能再 expand、download、preview 或 direct read。

P5 发布层级：

| 层级 | 开放对象 | Flag 策略 | 通过门禁 | 回滚权限和动作 |
|------|----------|-----------|----------|----------------|
| Internal Alpha | 开发者和内部测试团队 | 单子阶段 flag 手动开启；worker 可用 `RunOnce` 验证 | RED/GREEN、migration up/down、最小 fixture、flag off 行为 | 开发负责人关闭 flag，停止 worker，保留记录只读 |
| Pilot Team | 1-2 个明确团队空间 | 只开启该团队相关能力；默认不跨所有团队 | API 冒烟、审计可查、告警可查、回滚演练通过 | 团队 Admin/系统管理员可关闭子阶段 flag；P5c/P5d 停止新 job/run |
| GA | 符合条件团队逐步放量 | 按子阶段逐步放量，不允许 P5a-P5e 一次性全开 | 真实验证数据、MCP/HTTP 冒烟、告警阈值、runbook 完整 | SystemAdmin 关闭子阶段 flag；执行对应回滚 runbook |

P5 验证数据必须最少包含：

| 数据类别 | 最小 fixture |
|----------|--------------|
| 冲突候选 | 同团队相互矛盾知识 1 组、重复知识 1 组、非冲突高相似知识 1 组 |
| 版本 | 同一知识至少 3 个版本，包含标题变更、正文变更、标签变更 |
| URL 重抓 | 正常 HTTP 页面、重定向到内网、metadata IP、超大响应、非文本 content-type |
| 定时评测 | enabled schedule 1 个、disabled schedule 1 个、连续失败 schedule 1 个 |
| Organization 共享 | source team、target team、非成员用户、share pending/active/revoked 各 1 组 |

P5 fixture 细化：

- P5a Conflict：至少包含“相同正文重复知识 -> `duplicate`”、“同主题结论相反 -> `contradiction` 待人工确认”、“旧有效期覆盖新有效期 -> `outdated`”、“高相似但适用范围不同 -> 可 dismiss 的非冲突”。
- P5b Version：至少覆盖标题、正文、标签、状态四类变更；首次写入前 baseline；restore 后产生新版本并触发索引更新。
- P5c URL Refresh：除 SSRF 阻断外，还要有无变化、只改标题、正文更新、HTML 含 prompt injection、抓取失败五类正常产品样本；成功抓取只进入 `pending_review`。
- P5d Eval Schedule：enabled、disabled、连续失败三次、同一 `scheduled_for` 重试四类样本；失败原因对团队维护者可见。
- P5e Org Share：active shared 结果默认可展示 `id/title/source_team/source_kb/updated_at/quality_score/freshness_state`；正文、chunk、证据、文件和 metadata 默认不可见。

#### P5 Definition of Ready / Done

P5 每个子阶段必须单独达到 Ready 才能进入实现，单独达到 Done 才能声明完成。P5a-P5e 不共享完成状态；其中一个子阶段完成不代表 P5 完成。

| 子阶段 | Ready 条件 | Alpha Done | GA Done |
|--------|------------|------------|---------|
| P5a Conflict | P1b search 可复用；P3 freshness state 可读取；`duplicate/outdated/scope_overlap/contradiction` fixture 已建；AI 解释不作为自动处理依据 | 手动 check 可创建；worker 只生成候选；队列可列出；人工 `confirmed/dismissed/resolved` 状态流转可审计 | 同一未终态 pair 不重复；两个 worker 只一个成功 lease；flag off 后不再产生新 check/item；证据字段按 scope 裁剪 |
| P5b Version | 知识更新链路已找到统一 hook；必须版本化的写路径清单已确认；baseline fixture 已建 | 版本列表、相邻 diff、restore 可用；restore 生成新版本且不覆盖历史 | Web、旧 API、`suggest_to_team` apply、URL apply、freshness 处理和 restore 都有版本；SystemAdmin 读 personal 只有元数据 |
| P5c URL Refresh | P5b Version 已可用；safe fetch SSRF fixture 已建；最小 cron 和手动限流策略已确认 | 手动 job 可创建；抓取成功进入 `pending_review`；apply/reject 可审计；apply 生成版本 | schedule 可创建/停用；同一 slot 幂等；连续失败后延后或 disable；SSRF、DNS rebinding、超大响应、非文本类型全部阻断 |
| P5d Eval Schedule | P2 EvaluationService 可复用；dataset/run fixture 已建；cron 最小间隔确认 | 单个 schedule 能创建 P2 run；非法 cron 返回稳定 400；disabled 不触发 | 多实例不重复创建 run；同一 `schedule_id + scheduled_for` 幂等；连续失败后降频或停用；失败原因和审计可查 |
| P5e Org Share | 既有 Organization/member API 可复用；P1a ScopeResolver 和 P1b Search 可扩展；source/target team 角色 fixture 已建 | source team 可创建 share；target team 可 accept；active share 进入 shared scope；pending 不可搜索 | `allowed_fields` 服务端白名单裁剪；expand/download/preview/direct-id 均不越权；revoke 后新搜索和 direct-id 都不命中；org admin 不能替代团队 Admin/Owner |

P5 进入实现前必须满足：

- P1-P4 完成门禁已通过，特别是 ScopeResolver、SearchService、EvaluationService 和 Freshness state 可复用。
- P5a-P5e 每个子阶段都有独立 feature flag、回滚动作和最小 fixture。
- 每个子阶段第一张任务卡都能写出失败测试，失败原因必须是“生产代码未实现”，不是需求不清。
- direct-id API 已定义父资源回溯方式，例如 conflict item 回溯 KB、version/job 回溯 knowledge、share 回溯 org/source KB/target tenant。
- 会修改知识正文、标题、标签、状态或有效期的动作都先接入 VersionService；没有版本记录的 apply/restore 不允许上线。
- worker 型能力必须有 DB lease、失败阈值、审计或结构化日志和停用路径。
- 前端只消费后端状态和权限结果，不在前端自行判断能否越权操作。

#### P5 实施就绪结论

P5 可以进入实现阶段，但只能按 P5a-P5e 的独立任务卡推进。当前结论不是“P5 已完成”，而是“文档已足够让实现者选一张卡开始写 RED 测试”。多角色审查后的产品约束如下：

| 视角 | 结论 | 对实现的要求 |
|------|------|--------------|
| 产品 | P5 用户价值成立，但高级治理必须拆小发布 | 每张卡必须能对应一个维护者可观察的动作，例如查看冲突、恢复版本、确认 URL 更新、停用定时评测、撤销共享 |
| 架构 | P5 只能做治理层，不重写 P1-P4 主链路 | 修改知识正文必须走现有知识更新和索引链路；共享读取必须走 ScopeResolver；定时任务必须复用 P2 run |
| 后端 | P5 可实施，但 worker 和 direct-id 是主要风险 | 每个 worker 要有 DB lease、slot 幂等、停用路径；每个 direct-id API 第一行回溯父资源并重新解析 scope |
| 安全 | 默认关闭和字段裁剪是上线门禁 | feature flag fail-closed；shared scope 不含正文、chunk、证据、文件；审计只存元数据和脱敏摘要 |
| 现实检验 | 没有 API 冒烟、审计证据和回滚动作不得声明完成 | 每张卡交付时必须提交测试命令、fixture/API 调用、审计事件和关闭开关后的行为 |

P5 当前最小可实施切片：

| 切片 | 用户可见行为 | 最小验收数据 | Done 判定 |
|------|--------------|--------------|-----------|
| `P5c-5 URL refresh worker/audit` | URL 重抓后台只产生待确认更新，人工确认后才应用 | 正常 URL、内网 URL、metadata IP、重定向绕过、超大响应、非文本类型、连续失败 schedule | worker 可显式启动/停止；SSRF fixture 全阻断；schedule 只生成 job；apply 依赖 P5b Version；flag off 后 worker 不抓取 |
| `P5b-4 Version write hook inventory` | 任何会改变知识正文的治理动作都能在版本列表里追溯 | 旧知识 baseline、Web 更新、旧 API 更新、suggestion apply、URL apply、freshness、restore | 首次写入前生成 baseline；后续写入递增版本；restore 生成新版本；无版本记录的 apply/restore 不允许上线 |
| `P5a-4 Conflict gate/lifecycle` | 维护者能手动生成冲突候选并处理状态 | contradiction、duplicate、outdated、scope_overlap fixture 各 1 组 | AI 只生成解释和候选；feature flag off 不创建 check、不 lease；终态不可改回 open |
| `P5d-5 Eval schedule audit/API smoke` | 团队维护者能看到定时评测失败原因、下一次运行和系统暂停状态 | enabled schedule、disabled schedule、连续失败 schedule、同一 `scheduled_for` 重复触发 | schedule 更新/失败有审计；P2 run 与 schedule 推进具备原子性证据；HTTP 冒烟覆盖 200/400/403/409 |
| `P5e-8 Org share real smoke/front-end` | target team 成员真实 search/expand 共享知识，revoke 后同一 ID 不再可读 | source team Admin、target team Admin、非成员用户、share pending/active/revoked、`allowed_fields=["id","title"]` | HTTP/MCP 冒烟通过；revoke 后 search/expand/direct-id 0 命中；双方审计可查；需要 UI 时前端只消费后端状态 |

P5e 后续验收必须同时锁定两类证据：读取侧证据证明 `search_knowledge`、`expand_knowledge_result`、direct read、download、preview 在 active shared scope 下只返回 `allowed_fields`，且 revoke 后同一 ID 全部不命中；治理侧证据证明 create/accept/revoke 都有元数据审计，审计失败时 share 状态不推进。若创建者同时具备 source 和 target 团队 Admin/Owner，`CreateShare` 可以直接生成 `active` share，但必须在 `wika.org_share.created` 中记录 `new_status=active`，不得伪造一次没有真实 accept 调用的 `wika.org_share.accepted`。

## 六、安全需求

1. 日常 MCP 工具必须使用用户级身份令牌，不得复用租户级 API key 伪装 Admin；P1-P5 先实现 PAT，OAuth 后置。
2. PAT 必须绑定 `user_id`、`tenant_id`、scope、过期时间和撤销状态；未来 OAuth 必须映射到同一套 scope。
3. 个人知识正文、chunk、文件、图谱详情和评测内容仅 Owner 可读。
4. SystemAdmin 可以看元数据和统计，不得读取个人知识正文。
5. 个人知识越权访问返回 404，不暴露存在性。
6. 搜索、下载、预览、评测、保鲜、图谱等所有 API 都必须经过统一 scope 过滤。
7. AI 返回给用户或 Agent 的检索内容必须被视为不可信资料，不得当作系统指令执行。
8. `suggest_to_team` 的 AI 自动通过必须受风险规则约束，并保留审计。
9. 恶意知识内容中的指令、链接、脚本和凭据样式文本不得影响 AI 预审策略、权限判断和自动应用。
10. 旧 WeKnora API 与 Wika 新 API 使用同一套权限判断；不能只通过隐藏前端入口保护个人知识。
11. 自动 URL 重抓必须做 SSRF 防护，不能访问内网、元数据服务或非允许协议。
12. Organization 共享不得绕过来源团队授权和接收团队成员权限。

SystemAdmin 字段级边界：

| 数据 | SystemAdmin 可见性 |
|------|--------------------|
| 用户、空间、KB ID、名称、创建时间、成员数量 | 可读 |
| 知识 ID、标题、来源类型、状态、质量分、保鲜状态、访问统计 | 可读 |
| 知识正文、chunk、文件内容、snippet、证据、AI 修正文、评测 expected_answer、图谱证据文本 | 不可读 |
| 图谱实体名称、关系类型、数量统计 | 可读团队图谱元数据；个人图谱仅统计 |
| token 明文、token_hash、敏感规则命中原文 | 不可读；仅 token_prefix 和脱敏摘要 |

P5 字段级边界：

| 能力 | 可展示字段 | 禁止展示字段 |
|------|------------|--------------|
| Version list | version_no、title、content_hash、change_reason、created_by、created_at | 无权正文、old/new content、diff 原文 |
| Version diff | 有权用户可看 title/tag/status/content diff；SystemAdmin personal 仅元数据 | personal 正文 diff、snippet、AI 修正文原文 |
| URL refresh review | source_url_hash、final_url_hash、content_hash、diff_summary、failure_code | 抓取正文全文、内网地址明文、恶意 HTML 原文 |
| Eval schedule | cron hash 或展示值、next_run_at、last_run_id、failure_code、consecutive_failures | expected_answer、case 正文、无权 snippet |
| Org shared result | id、title、source_team、source_kb、updated_at、quality_score、freshness_state | content、chunk、evidence_text、file、metadata、diff |

## 七、观测与指标

| 环节 | 指标 |
|------|------|
| MCP | 调用量、成功率、P95、入库转化率 |
| 入库 | parse/index 成功率、入库耗时、质量分分布、重复率 |
| 检索 | no-hit 率、top1/top5 点击率、引用率、RRF 各路贡献 |
| 团队流转 | 自动通过率、待确认率、不通过率、人工覆盖率 |
| 评测 | MRR、Recall@5、NDCG@5、低分 QA 数 |
| 保鲜 | 过期知识数、低置信知识数、长期未命中知识数 |
| 高级治理 | open conflict 数、restore 次数、URL refresh 失败率、due schedule 延迟、active/revoked share 数 |
| 成本 | LLM token、embedding 次数、rerank 次数、图谱抽取次数 |

P5 worker 告警指标：

| 指标 | 标签 | 告警阈值 | 动作 |
|------|------|----------|------|
| `wika_worker_due_lag_seconds` | stage、worker、tenant_id | P5c/P5d due lag > 10 分钟持续 15 分钟 | 检查 flag、worker 进程、DB lease、队列积压 |
| `wika_worker_lease_conflict_total` | stage、worker | 5 分钟内异常升高 | 检查多实例竞争和 lease duration |
| `wika_worker_disabled_total` | stage、reason | flag off 后仍出现 job/run 创建为 Critical | 立即停 worker，检查 fail-closed 测试 |
| `wika_url_refresh_failure_total` | failure_code、tenant_id | 同一 failure_code 1 小时内 > 20 或失败率 > 30% | 降低 schedule 频率，检查 fetcher/网络/SSRF 规则 |
| `wika_eval_schedule_failed_total` | failure_code、tenant_id | 连续失败 schedule > 0 且未 disable | 检查 P2 EvaluationService 和 dataset 可用性 |
| `wika_org_share_revoked_hit_total` | source_tenant_id、target_tenant_id | 任意值 > 0 为 Critical | 关闭 shared scope，检查 resolver/cache |

## 八、阶段验收标准

### P1 基础闭环验收

1. 用户 A/B 注册后各有独立个人空间，互相不能通过 API 搜索或读取对方个人知识。
2. A 能通过 Web 创建知识，也能通过 MCP `push_knowledge` 写入知识。
3. A 能通过 MCP `search_knowledge` 搜到个人知识和已加入团队知识。
4. A 加入团队后，个人知识不会自动泄露给团队成员 B。
5. A 发起 `suggest_to_team` 后，系统返回 `通过 / 待确认 / 不通过` 之一，并附 AI 修正版和理由。
6. 团队默认未开启自动应用时，AI 判定为通过的内容不会自动进入团队；开启自动应用且安全门禁通过后，内容能按规则自动进入团队。
7. 人工可以覆盖任意预审结果，并修改内容后发布到团队。
8. 普通用户创建知识库只需要名称、描述、类型，高级配置默认收起。
9. 使用旧知识读取、搜索、下载、预览接口访问他人个人空间内容时返回 404 或无结果。

### P2 质量闭环验收

1. 团队知识库能创建、导入、导出黄金 QA。
2. 正式评测 run 持久化 run 指标和 case 明细。
3. 单条 QA dry-run 可展示召回结果，不污染正式趋势。
4. `expected_knowledge_ids` 或 `expected_chunk_ids` 缺失时不得计入正式指标。
5. A/B 越权用户不能通过评测接口导出或查看他人个人正文、snippet 或 expected answer。

### P3 保鲜闭环验收

1. 过期、将过期、长期未访问、低质量、低置信均可构造出保鲜项。
2. 保鲜项支持更新、延长有效期、废弃、忽略、重新推荐到团队。
3. 检索热路径不直接更新 `knowledges` 主表，访问统计通过事件或日聚合进入保鲜。
4. 保鲜处理动作有操作者、前后状态和审计记录。

### P4 图谱发现验收

1. 团队成员可浏览团队实体、关系、来源知识和证据摘要。
2. 个人图谱只允许 Owner 查看；SystemAdmin 不可读取个人图谱证据正文。
3. 图谱增强检索可开启/关闭，并能解释图谱召回贡献。
4. 图谱服务失败时主搜索降级成功，API 返回降级标识而不是 500。

### P5 高级治理验收

1. 冲突检测能生成候选、解释、状态流转和人工处理结果。
2. 知识版本可查看 diff，可恢复，恢复生成新版本并保留历史。
3. URL 重抓具备 SSRF 防护 fixture，通过后只生成待确认更新。
4. 定时评测可配置、运行、失败重试/降频，并保留 run 明细。
5. Organization 共享可以授权、引用检索、撤销；撤销后新检索不再命中。
6. P5a-P5e 关闭任一 feature flag 后，对应 worker 不再领取新任务，对应写 API 不再产生新状态。
7. 所有 P5 审计只包含元数据、hash 和脱敏摘要，不包含正文、snippet、diff 全文、抓取正文或 expected answer。
8. 每个 P5 子阶段必须按独立任务卡提交 RED 测试、最小实现、回归命令、API 冒烟和回滚动作；一个子阶段完成不代表整个 P5 完成。

## 九、后续增强

- 自动推荐高价值个人知识到团队。
- 跨工具工作流：从 IDE、IM、浏览器插件触发统一知识生产。
- 更细粒度的 Organization 数据策略，例如字段级脱敏共享和只读快照。
