# Wika 个性化改造

## 项目定位

Wika 是从 [WeKnora](https://github.com/Tencent/WeKnora) fork 的知识闭环系统，
在其基础之上新增个人/团队空间、AI 工具生产知识、团队沉淀、评测与保鲜等能力。

**与 WeKnora 的关系**：WeKnora 定位是「企业 AI 知识库问答平台」，Wika 定位是「知识中枢（Knowledge Hub）」——知识的基础设施层，不绑定 AI 应用形态。

## 文档索引

| 文档 | 说明 |
|------|------|
| [requirements.md](./requirements.md) | 改造需求方案：知识生产、评测、存储、消费、团队流转与保鲜闭环 |
| [tech-plan.md](./tech-plan.md) | 技术实现方案：Space、MCP、混合检索、AI 预审、评测与保鲜分期实现 |

## 阅读路径

本文档面向准备实现 Wika 改造的开发者和后续 AI 代码代理。读完后应能判断当前该做哪个阶段、先写哪些测试、哪些边界不能突破。

1. 先读 [requirements.md](./requirements.md) 的目标、阶段范围和“阶段实施故事与验证数据”，确认每期用户价值和验收样例。
2. 再读 [tech-plan.md](./tech-plan.md) 的 P0 ADR、数据模型、服务设计、API 契约和分期实施。
3. 编码时只进入当前阶段任务卡，不提前实现后续阶段能力。
4. 每期完成前按 [tech-plan.md](./tech-plan.md) 的“完成门禁”提交验证证据。

## 当前实施边界

当前文档已补齐到 P0-P5 均可拆分实施的状态，其中 P5 已拆成 P5a-P5e 子阶段、任务卡、RED 测试包、迁移约束、worker lease、API 状态机、审计事件、灰度回滚和验收证据。P5 进入实现前还必须遵守已收口的硬门禁：feature flag fail-closed、worker 显式 lifecycle、审计同事务、schedule slot 幂等、URL 重抓成本上限、版本 baseline、Organization 复用既有组织表且团队 Admin/Owner 始终是数据授权源头。实现仍必须按阶段推进，不能因为后续阶段已有方案就跳过前置安全和数据门禁。

当前仓库已有 P5a-P5e 的迁移、types、service/store 与部分 handler 接入，且 P5e expand/direct-id/audit 回归、P5d eval schedule worker lifecycle 已落地。继续 P5 时不要重复建 `000097-000102` 迁移；先按 [tech-plan.md](./tech-plan.md) 的“当前仓库 P5 实现状态”和对应任务卡补 URL refresh worker、Version 真实写路径 hook、Conflict gate/lifecycle、Eval schedule 审计/API 冒烟和真实阶段验证数据。当前 hardening 卡默认不交付前端；只有任务卡明确写“前端入口”时才改 `frontend/src/views/wika/**`。

P5 当前可以进入“单卡实现”，但不能把 P5 作为一个整体宣布完成。实施者必须先选定一张任务卡、写出 RED 测试，再进入最小实现。当前优先级：

| 顺序 | 任务卡 | 交付目标 | 禁止混入 |
|------|--------|----------|----------|
| 1 | `P5c-5 URL refresh worker/audit` | 生产 worker lifecycle、container 注册、flag off 不抓取、连续失败后 schedule 延后或停用并审计 | 绕过 SSRF、抓取后直接覆盖正文、绕过 P5b Version |
| 2 | `P5b-4 Version write hook inventory` | Web、旧 API、`suggest_to_team` apply、URL apply、freshness、restore 写路径全部进入版本链路 | 只在 handler 手动调用 `RecordVersion`、restore 覆盖历史 |
| 3 | `P5a-4 Conflict gate/lifecycle` | feature flag fail-closed、worker lifecycle、终态不可回滚、审计失败不推进状态 | AI 自动合并、自动删除或直接改知识正文 |
| 4 | `P5d-5 Eval schedule audit/API smoke` | 已有 worker 基础上补审计事件、HTTP 冒烟、失败告警和真实 schedule fixture | 重写 P2 EvaluationService、无限失败重试 |
| 5 | `P5 real-stage verification data` | 为 P5a-P5e 各准备 1 组真实 DB/API/MCP 验证数据，证明可观测、可回滚 | 只用单测结果宣布 P5 完成 |

P5 当前开工卡片：

| 当前卡 | 本卡范围 | 首个 RED | 最小命令 | 冒烟与交付证据 |
|--------|----------|----------|----------|----------------|
| `P5c-5 URL refresh worker/audit` | 新增 `urlrefresh.Worker`、container 启停、失败 backoff/disable 审计；不改 apply 语义 | `internal/wika/governance/urlrefresh/worker_test.go::TestWorkerCreatesDueJobsAndRunsThemUntilStopped`：enabled flag 下先 `RunDueSchedules` 再逐个 `RunJob`，Stop 后不再调用 | `go test ./internal/wika/governance/urlrefresh ./internal/container -run 'TestWorker|URLRefresh' -count=1` | flag off 不调用 due/job；`RunOnce` 或等价手动触发；`wika.url_refresh.job_failed/schedule_disabled` 不含正文/URL 明文 |
| `P5b-4 Version write hook inventory` | 先盘点真实写路径并接统一 hook；不新增版本页面 | `internal/wika/governance/version/service_test.go` 或写路径适配测试：任一必须版本化路径缺版本即失败 | `go test ./internal/wika/governance/version ./internal/handler -count=1` | baseline、新版本、restore 新版本；无权 diff 不返回正文；hook 防递归 |
| `P5a-4 Conflict gate/lifecycle` | 补 feature gate、worker lifecycle、状态和审计回滚；不做自动 trigger GA | `internal/wika/governance/conflict/worker_test.go`：flag off 不 lease，终态 item 不再推进 | `go test ./internal/wika/governance/conflict ./internal/handler ./internal/container -count=1` | manual check、candidate、resolve、flag off、audit fail rollback |
| `P5d-5 Eval schedule audit/API smoke` | 在已落地 worker 上补审计、API 冒烟、失败提示；不复制评测指标 | `internal/wika/governance/evalschedule/service_test.go`：schedule 更新/失败写审计且 audit 失败不推进状态 | `go test ./internal/wika/governance/evalschedule ./internal/handler ./internal/container -count=1` | `/api/v1/wika/kb/:id/eval/schedules` 200/400/403/409；due lag、failure disable 可查 |

P5 开工前证据清单：

- P1-P4 门禁如果没有真实环境证据，可以先做 P5 单卡 package 级实现，但不得声明 P5 子阶段 GA。
- 进入 P5 worker 或共享读取相关卡前，至少要能提供 ScopeResolver、SearchService、EvaluationService、Freshness state 对应 package 回归命令。
- 涉及 MCP 消费路径时，使用 `WEKNORA_PAT`，不使用 `WEKNORA_API_KEY` fallback。
- 每张卡收尾必须把 RED 失败摘要、GREEN 命令、API/MCP 冒烟、审计事件、flag off 行为和回滚动作写进交付说明。

实施入口：

1. 先完成 [tech-plan.md](./tech-plan.md) 的 P0 ADR，并把 ADR-01 到 ADR-10 作为编码前不可变决策。
2. P1 拆成 P1a/P1b/P1c：先做空间、默认 KB、用户级 token 和旧 API scope；再做入库/检索；最后做 `suggest_to_team`。
3. P2-P5 每期都有独立数据模型、API、权限、安全和验收门禁；P5 子阶段也必须独立 feature flag、独立回滚、独立验收。
4. 所有阶段都必须满足：个人正文不被 SystemAdmin 读取、旧 WeKnora API 不绕过 personal scope、AI 输出不绕过确定性安全规则。

P5 快速开工入口：

- 先确认 P1-P4 门禁已通过，再按 [tech-plan.md](./tech-plan.md) 的“P5 Implementation Map”“P0-P5 任务卡索引”“P5 RED 测试包”拆 PR。
- 开工前先读 [tech-plan.md](./tech-plan.md) 的“P5 每卡 TDD 执行模板”“P5 子阶段最小切片边界”和“P5 通用实现契约”，确认当前 PR 只覆盖一张任务卡。
- 每个 P5 子阶段先写 migration/type 约束 RED 测试，再写 store/service 状态机和权限测试，最后补 handler/router/container 测试。
- P5b Version 是 P5c URL Refresh apply 的前置；P5c 不得绕过 VersionService 直接改正文。
- P5e 必须先让 ScopeResolver 输出 `shared scope` 和 `allowed_fields`，再接 SearchService、expand、download、preview 和 direct-id 读取；当前这些基础读取回归已落地，后续 P5e 只补真实 HTTP/MCP 冒烟、revoke 传播 SLA 和必要前端队列入口。
- 选择 P5e 后续验收卡时，直接按 [tech-plan.md](./tech-plan.md) 的“P5e 后续验收规格”执行；其中已固定真实 HTTP 路由、MCP PAT 配置、最小 fixture、审计事件语义和同事务验收。
- P5a/P5c/P5d worker 必须先通过 feature flag fail-closed、DB lease、schedule slot 幂等和停用路径测试，不能先上线单实例假设。

P5 实施读法：

| 读什么 | 解决什么问题 | 必须拿到的输出 |
|--------|--------------|----------------|
| [requirements.md](./requirements.md) 的 P5 子阶段需求、发布矩阵和 Definition of Ready/Done | 明确用户价值、非目标、灰度和验收边界 | 当前 PR 属于 P5a-P5e 哪个子阶段，Alpha/GA 分别验什么 |
| [tech-plan.md](./tech-plan.md) 的 P5 API 契约、状态机、迁移约束和 RED 测试包 | 明确 handler、service、store、worker、migration 怎么写 | 请求/响应、状态枚举、错误码、审计事件、测试文件和命令 |
| [tech-plan.md](./tech-plan.md) 的 P5 每卡证据模板 | 明确完成时提交什么证据 | migration up/down、单测命令、API 冒烟、审计事件和回滚动作 |

P5 开工前最小清单：

- 已选择唯一任务卡，例如 `P5c-3 URL review/apply`；同 PR 不混入其他子阶段能力。
- 已确认 feature flag key、默认关闭行为、flag 读取失败时的 fail-closed 测试。
- 已准备最小 fixture，并能让首个 RED 测试失败在“生产代码未实现”，不是需求或测试数据不清。
- 已确认本卡涉及的状态机、审计事件和错误码；安全策略失败不能落成 500。
- 涉及 worker 或 schedule 的卡，已写 DB lease、slot 幂等、失败降频和 worker lifecycle 测试。
- 涉及知识正文变更的卡，已接入 VersionService；没有版本记录不得上线 apply/restore。

实施前检查：

- 当前 PR 属于 P1a/P1b/P1c/P2/P3/P4/P5a-P5e 中哪一张任务卡。
- 已写出对应 RED 测试，且失败原因符合预期。
- 需要改旧 WeKnora API 时，同 PR 包含旧接口越权测试。
- MCP 日常工具使用用户级 PAT，不复用租户 API key。
- P5 能力默认不在 P1-P4 暗中开启。
- P5 worker 使用 DB lease 和幂等状态机，不能用进程内锁或单实例假设。
- P5e 不新建平行 Organization 主表；复用既有 `organizations`、`organization_tenant_members`，只新增 Wika share 授权层。

关键前置决策：

- Space 复用 Tenant，个人空间不通过 KB visibility 表达。
- 日常 MCP 当前实现使用用户级 PAT，不复用租户 API key；OAuth 是后续同等身份形态。
- `search_knowledge` 先做权限 scope，再做跨 KB 混合检索。
- `suggest_to_team` 默认不自动发布到团队；只有团队策略显式开启且通过确定性安全门禁时，AI 通过结果才可自动应用。
- 旧 WeKnora KB/knowledge/search/download/preview API 必须接入 personal scope，不能只保护 Wika 新 API。

## 上游合并策略

所有改动遵循以下原则以减少与上游 WeKnora 的合并冲突：

1. **新文件优先** — 新增功能尽量放在新文件/新目录中，不修改已有文件
2. **门面编排** — Wika 新能力优先放在 `internal/wika/**`，复用现有服务，不复制领域逻辑
3. **独立迁移** — 数据库迁移使用高位编号（090+），不与上游迁移冲突
4. **独立路由** — 新增路由挂载到独立路由组，必要时仅在路由注册处末尾追加
5. **独立前端页面** — 新增页面作为独立 Vue 组件，路由懒加载
6. **配置不改 YAML 结构** — 新增运行时默认值优先进入 `system_settings` registry；必要的只读 fallback 可保留环境变量，不修改 `config/config.yaml` 结构

如须修改已有文件，在 [tech-plan.md](./tech-plan.md) 中明确标注冲突风险等级和应对策略。
