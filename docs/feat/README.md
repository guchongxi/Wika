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

## 当前实施边界

当前文档已收敛到可进入实现拆分的状态，但编码前必须先完成 [tech-plan.md](./tech-plan.md) 中 P0 ADR：

- Space 复用 Tenant，个人空间不通过 KB visibility 表达。
- 日常 MCP 使用用户级 PAT/OAuth，不复用租户 API key。
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
