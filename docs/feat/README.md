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

当前文档已补齐到 P0-P5 均可拆分实施的状态。实现仍必须按阶段推进，不能因为后续阶段已有方案就跳过前置安全和数据门禁。

实施入口：

1. 先完成 [tech-plan.md](./tech-plan.md) 的 P0 ADR，并把 ADR-01 到 ADR-10 作为编码前不可变决策。
2. P1 拆成 P1a/P1b/P1c：先做空间、默认 KB、用户级 token 和旧 API scope；再做入库/检索；最后做 `suggest_to_team`。
3. P2-P5 每期都有独立数据模型、API、权限、安全和验收门禁；只有上一期门禁通过，下一期才能进入编码。
4. 所有阶段都必须满足：个人正文不被 SystemAdmin 读取、旧 WeKnora API 不绕过 personal scope、AI 输出不绕过确定性安全规则。

实施前检查：

- 当前 PR 属于 P1a/P1b/P1c/P2/P3/P4/P5a-P5e 中哪一张任务卡。
- 已写出对应 RED 测试，且失败原因符合预期。
- 需要改旧 WeKnora API 时，同 PR 包含旧接口越权测试。
- MCP 日常工具使用用户级 PAT，不复用租户 API key。
- P5 能力默认不在 P1-P4 暗中开启。

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
