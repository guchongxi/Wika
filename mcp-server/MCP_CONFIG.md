# 使用 uv 运行 WeKnora MCP 服务器

> 本文只适用于本地开发调试或旧版 stdio 客户端兼容。Wika 正式用户接入应使用云端 Remote MCP 地址，不需要本地安装本项目；配置方式见 [MCP/API 知识生产接入](../docs/MCP知识生产接入.md)。
>
> 默认云端入口不传工具集请求头时只开放 `push_knowledge`、`search_knowledge`、`expand_knowledge_result`、`get_my_knowledge`、`suggest_to_team`。租户、模型、知识库等管理工具应通过 Remote MCP 请求头 `X-Wika-MCP-Toolset: admin` 请求，并由后端校验 `mcp:admin` scope 和管理员角色。

> 更推荐使用`uv`来运行基于python的MCP服务。

## 1. 安装 uv

```bash
# macOS/Linux
curl -LsSf https://astral.sh/uv/install.sh | sh

# 或使用 Homebrew (macOS)
brew install uv

# Windows
powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"
```

## 2. MCP 客户端配置

### Claude Desktop 配置

在 Claude Desktop 设置中添加:

```json
{
  "mcpServers": {
    "weknora": {
      "args": [
        "--directory",
        "/path/WeKnora/mcp-server",
        "run",
        "run_server.py"
      ],
      "command": "uv",
      "env": {
        "WEKNORA_API_KEY": "your_api_key_here",
        "WEKNORA_BASE_URL": "http://localhost:8080/api/v1"
      }
    }
  }
}
```

### Cursor 配置

在 Cursor 中，编辑 MCP 配置文件 (通常在 `~/.cursor/mcp-config.json`):

```json
{
  "mcpServers": {
    "weknora": {
      "command": "uv",
      "args": [
        "--directory",
        "/path/WeKnora/mcp-server",
        "run",
        "run_server.py"
      ],
      "env": {
        "WEKNORA_API_KEY": "your_api_key_here",
        "WEKNORA_BASE_URL": "http://localhost:8080/api/v1"
      }
    }
  }
}
```

### KiloCode 配置

对于 KiloCode 或其他支持 MCP 的编辑器，配置如下:

```json
{
  "mcpServers": {
    "weknora": {
      "command": "uv",
      "args": [
        "--directory",
        "/path/WeKnora/mcp-server",
        "run",
        "run_server.py"
      ],
      "env": {
        "WEKNORA_API_KEY": "your_api_key_here",
        "WEKNORA_BASE_URL": "http://localhost:8080/api/v1"
      }
    }
  }
}
```

### 其他 MCP 客户端

对于一般 MCP 客户端配置:

```json
{
  "mcpServers": {
    "weknora": {
      "command": "uv",
      "args": [
        "--directory",
        "/path/WeKnora/mcp-server",
        "run",
        "run_server.py"
      ],
      "env": {
        "WEKNORA_API_KEY": "your_api_key_here",
        "WEKNORA_BASE_URL": "http://localhost:8080/api/v1"
      }
    }
  }
}
```
