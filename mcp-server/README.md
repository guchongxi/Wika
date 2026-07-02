# WeKnora MCP Server

这是一个 Model Context Protocol (MCP) 服务器，提供对 WeKnora 知识管理 API 的访问。

> 在 Wika 云端集中架构中，本目录是服务端 MCP 组件，不是终端用户必须安装的客户端依赖。终端用户应直接使用 Wika 提供的 Remote MCP 地址和个人 PAT，见 [MCP/API 知识生产接入](../docs/MCP知识生产接入.md)。

## 服务端/开发调试快速开始

生产用户入口建议使用 HTTP transport。默认 `dynamic` 模式下，不传工具集请求头时只开放日常知识生产工具；客户端请求 `X-Wika-MCP-Toolset: admin` 时，服务端会向 Wika API 校验 `mcp:admin` scope 和管理员角色。

### 1. 安装依赖
```bash
pip install -r requirements.txt
```

### 2. 配置环境变量
```bash
# Linux/macOS
export WEKNORA_BASE_URL="http://localhost:8080/api/v1"
export WEKNORA_MCP_TOOLSET="dynamic"

# Windows PowerShell
$env:WEKNORA_BASE_URL="http://localhost:8080/api/v1"
$env:WEKNORA_MCP_TOOLSET="dynamic"

# Windows CMD
set WEKNORA_BASE_URL=http://localhost:8080/api/v1
set WEKNORA_MCP_TOOLSET=dynamic
```

云端 dynamic 模式不配置固定 `WEKNORA_PAT`，由每个请求的 `Authorization: Bearer wika_pat_xxx` 透传用户身份。若要彻底禁用管理工具，设置 `WEKNORA_MCP_TOOLSET=daily`；只有可信管理员/内网部署需要旧版兼容工具时，才设置 `WEKNORA_MCP_TOOLSET=all` 并配置 `WEKNORA_API_KEY` 或 `WEKNORA_PAT`。

### 3. 运行服务器

**推荐方式：**
```bash
python main.py --transport http --host 0.0.0.0 --port 8000
```

**其他运行方式：**
```bash
# 使用原始启动脚本
python run_server.py

# 使用便捷脚本
python run.py

# 直接运行服务器模块
python weknora_mcp_server.py

# 作为 Python 模块运行
python -m weknora_mcp_server
```

### 4. 命令行选项
```bash
python main.py --help                 # 显示帮助信息
python main.py --check-only           # 仅检查环境配置
python main.py --verbose              # 启用详细日志
python main.py --version              # 显示版本信息
```

## 安装为 Python 包

### 开发模式安装
```bash
pip install -e .
```

安装后可以使用命令行工具：
```bash
weknora-mcp-server
# 或
weknora-server
```

### 生产模式安装
```bash
pip install .
```

### 构建分发包
```bash
# 使用 setuptools
python setup.py sdist bdist_wheel

# 使用现代构建工具
pip install build
python -m build
```

## 测试模组

运行测试脚本验证模组是否正常工作：
```bash
python test_module.py
```

## 功能特性

默认 `WEKNORA_MCP_TOOLSET=dynamic` 且客户端不传工具集请求头时，只暴露日常知识生产工具：

- `push_knowledge`
- `search_knowledge`
- `expand_knowledge_result`
- `get_my_knowledge`
- `suggest_to_team`

客户端传 `X-Wika-MCP-Toolset: admin` 且后端授权通过，或设置 `WEKNORA_MCP_TOOLSET=all` 后，才暴露以下管理工具：

### 租户管理
- `create_tenant` - 创建新租户
- `list_tenants` - 列出所有租户

### 知识库管理
- `create_knowledge_base` - 创建知识库
- `list_knowledge_bases` - 列出知识库
- `get_knowledge_base` - 获取知识库详情
- `delete_knowledge_base` - 删除知识库
- `hybrid_search` - 混合搜索

### 知识管理
- `create_knowledge_from_url` - 从 URL 创建知识
- `list_knowledge` - 列出知识
- `get_knowledge` - 获取知识详情
- `delete_knowledge` - 删除知识

### 模型管理
- `create_model` - 创建模型
- `list_models` - 列出模型
- `get_model` - 获取模型详情

### 会话管理
- `create_session` - 创建聊天会话
- `get_session` - 获取会话详情
- `list_sessions` - 列出会话
- `delete_session` - 删除会话

### 聊天功能
- `chat` - 发送聊天消息

### 块管理
- `list_chunks` - 列出知识块
- `delete_chunk` - 删除知识块

## 故障排除

如果遇到导入错误，请确保：
1. 已安装所有必需的依赖包
2. Python 版本兼容（推荐 3.10+）
3. 没有文件名冲突（避免使用 `mcp.py` 作为文件名）

## 调用效果

<img width="950" height="2063" alt="118d078426f42f3d4983c13386085d7f" src="https://github.com/user-attachments/assets/09111ec8-0489-415c-969d-aa3835778e14" />
