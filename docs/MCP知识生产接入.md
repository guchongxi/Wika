# MCP/API 知识生产接入

本文面向希望用 AI 工具把知识写入 Wika 的用户和管理员。

## 使用方式

Wika 日常知识生产工具使用用户级 PAT，不使用租户 API Key。

1. 打开设置 -> AI 工具接入。
2. 创建日常 Token，scope 至少选择：
   - `knowledge:push`
   - `knowledge:read`
   - `knowledge:search`
   管理员如需在 AI 工具中调用管理型 MCP 工具，另建或补充包含 `mcp:admin` 的管理员 PAT。
3. 复制创建时唯一一次显示的 `wika_pat_xxx`。
4. 在 AI 工具中配置 Wika 提供的云端 Remote MCP 地址，并把 PAT 放入请求头。

终端用户不需要本地安装 Wika 项目，也不需要本地运行 `weknora_mcp_server`。

## Remote MCP 配置示例

生产环境由 Wika 服务端统一托管 MCP 服务，推荐通过网关暴露为：

```text
https://<wika-domain>/mcp
```

本地开发若单独启动 `mcp-server` 容器，默认地址为：

```text
http://localhost:8082/mcp
```

AI 工具侧只需要配置 Remote MCP URL 和用户自己的 PAT。不同客户端字段名可能略有差异，日常模式核心是 `url` 和 `Authorization`：

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

管理员需要管理工具时，客户端显式增加 `X-Wika-MCP-Toolset: admin`。服务端仍会校验 PAT 是否包含 `mcp:admin` scope，以及 PAT 所属用户是否为当前租户 Admin/Owner 或系统管理员；不通过则返回无权限。

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

Remote MCP 收到用户请求后，会把请求头中的 PAT 透传给 Wika daily API：

```http
Authorization: Bearer wika_pat_xxx
```

不要用 `WEKNORA_API_KEY` 调用 `/api/v1/wika/knowledge/push` 等 daily routes；这些路由会拒绝租户 API Key。

云端用户入口默认不传工具集请求头，因此只暴露日常知识生产工具：

- `push_knowledge`
- `search_knowledge`
- `expand_knowledge_result`
- `get_my_knowledge`
- `suggest_to_team`

租户、知识库、模型、会话等管理型 MCP 工具只在客户端请求 `X-Wika-MCP-Toolset: admin` 且后端授权通过时暴露。

## 服务端部署说明

`mcp-server/weknora_mcp_server.py` 是 Wika 云端 MCP 服务组件，不是终端用户必须安装的客户端依赖。

部署时使用 HTTP transport：

```bash
weknora-mcp-server --transport http --host 0.0.0.0 --port 8000
```

容器镜像默认也是 HTTP transport。`docker-compose.yml` 中的 `mcp` 服务会把容器 `8000` 端口映射到宿主 `MCP_PORT`，默认 `8082`。

多用户云端模式不要给 MCP 服务配置固定 `WEKNORA_PAT`。MCP 服务应读取每个请求的 `Authorization: Bearer wika_pat_xxx`，并按该用户身份调用 Wika API，这样调用统计才能归属到对应用户和 token。

生产部署建议保持默认 `WEKNORA_MCP_TOOLSET=dynamic`：客户端可请求 daily/admin，admin 必须通过后端授权。若某个部署完全禁止 MCP 管理工具，可设置 `WEKNORA_MCP_TOOLSET=daily`。只有可信管理员内网需要旧版无请求级校验的兼容工具时，才设置 `WEKNORA_MCP_TOOLSET=all` 并配置对应的服务端凭据。

本地 `stdio` 启动方式只保留为开发调试或兼容旧客户端，不作为推荐接入路径。

## API 调用示例

```bash
curl -sS http://localhost:8080/api/v1/wika/knowledge/push \
  -H 'Authorization: Bearer wika_pat_xxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "title": "本地 MCP/API 入库验收",
    "content": "这条知识用于验证 Wika PAT 可以写入个人默认知识库。",
    "source": "local-smoke",
    "tags": ["smoke", "mcp"],
    "idempotency_key": "smoke-001"
  }'
```

成功响应会包含：

- `knowledge_id`
- `tenant_id`
- `kb_id`
- `source_channel`
- `created_at`
- `status`

重复使用相同 `idempotency_key` 时，系统返回已有 `knowledge_id`，不会重复创建知识。

## 验证入库

```bash
curl -sS 'http://localhost:8080/api/v1/wika/knowledge/mine?limit=5' \
  -H 'Authorization: Bearer wika_pat_xxx'
```

如果能在返回结果中看到刚写入的 `knowledge_id`，说明知识已进入个人默认知识库。

## MCP Inspector 验收

项目提供可重复执行的 MCP Inspector smoke 脚本：

```bash
scripts/mcp-inspector-smoke.sh daily
scripts/mcp-inspector-smoke.sh admin
scripts/mcp-inspector-smoke.sh all
```

默认地址：

- MCP URL：`http://localhost:8082/mcp`
- Wika API：`http://localhost:8080/api/v1`

如需覆盖地址：

```bash
MCP_URL=http://localhost:8082/mcp \
WIKA_API_BASE=http://localhost:8080/api/v1 \
scripts/mcp-inspector-smoke.sh daily
```

脚本依赖 `curl`、`jq`、`npx`、`python3`。Inspector 由脚本通过 `npx -y @modelcontextprotocol/inspector` 调用。

### 使用已有 PAT

只测 daily 工具：

```bash
WIKA_DAILY_PAT=wika_pat_xxx \
scripts/mcp-inspector-smoke.sh daily
```

测管理员工具集时需要同时准备一个 daily PAT 和一个包含 `mcp:admin` scope 的管理员 PAT。daily PAT 用于验证“缺少 `mcp:admin` 时必须被拒绝”。

```bash
WIKA_DAILY_PAT=wika_pat_daily_xxx \
WIKA_ADMIN_PAT=wika_pat_admin_xxx \
scripts/mcp-inspector-smoke.sh admin
```

### 自动创建临时 PAT

如果提供登录后的用户 JWT，脚本会自动创建临时 PAT，测试结束后自动撤销。admin 模式要求该 JWT 所属用户是当前租户 Admin/Owner，且允许创建 `mcp:admin` scope。

```bash
WIKA_JWT='<登录后的 JWT>' \
scripts/mcp-inspector-smoke.sh daily

WIKA_JWT='<管理员登录后的 JWT>' \
scripts/mcp-inspector-smoke.sh admin
```

如果需要保留临时 PAT 做后续手动排查，可显式设置：

```bash
KEEP_TEST_TOKENS=true \
WIKA_JWT='<登录后的 JWT>' \
scripts/mcp-inspector-smoke.sh all
```

### 验收覆盖范围

daily 模式会验证：

- `tools/list` 只暴露 `push_knowledge`、`search_knowledge`、`expand_knowledge_result`、`get_my_knowledge`、`suggest_to_team`
- `push_knowledge` 可以写入个人默认知识库
- `get_my_knowledge` 可以读到刚写入的知识
- `search_knowledge` 可以检索到刚写入的知识
- `expand_knowledge_result` 可以展开刚写入的知识

admin 模式会验证：

- 有 admin PAT 但不传 `X-Wika-MCP-Toolset: admin` 时，仍只暴露 daily 工具
- 无 token、无效 token、缺少 `mcp:admin` scope 的 token 请求 admin 工具集时均返回无权限
- 合法 admin PAT 加 `X-Wika-MCP-Toolset: admin` 后可以看到管理工具
- 合法 admin PAT 可以调用 `list_knowledge_bases`
- 不传 admin header 时不能直接调用管理工具绕过工具集限制

## 调用统计

普通用户可在设置 -> AI 工具接入 -> 调用统计中查看自己的 token 调用情况。

租户 Admin/Owner 可切换到“租户全部”，查看当前租户内所有用户的 token 调用情况。

统计只记录脱敏元数据：

- token id、名称、prefix
- owner
- tool/API
- 状态码
- 成功/失败
- 耗时
- `knowledge_id`

统计不会保存 token 明文、token hash、知识正文或 evidence 原文。

## 常见错误

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 401 invalid Wika PAT | Token 不存在、过期或已撤销 | 重新创建 PAT |
| 403 insufficient Wika PAT scope | Token 缺少当前工具所需 scope | 创建包含对应 scope 的 PAT |
| 403 Wika PAT routes require user JWT or Wika PAT | 使用了租户 API Key | 改用用户 PAT，并通过 `Authorization: Bearer wika_pat_xxx` 请求头传入 |
| 找不到写入的知识 | 当前用户没有个人默认知识库或入库失败 | 重新登录后再试，或联系管理员检查个人空间初始化 |
