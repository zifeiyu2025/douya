# 外部编程工具接入说明

豆芽通过启动 `llama-server`（llama.cpp 官方 server）对外提供 **OpenAI 兼容的 HTTP API**。暴露服务器地址后，Claude Code、Codex、OpenCoder 及任意 OpenAI 兼容客户端可直接连接，把豆芽当作本地模型服务使用。

---

## 一、前置准备

在接入外部工具前，需在豆芽中完成以下三项配置：

### 1. 开启局域网访问（跨机器接入时需要）
- 打开豆芽 → 设置 → "服务 API KEY" 区块
- 开启 **"暴露服务器地址"** 开关
- 提示"需重启服务生效"后，重启豆芽应用

> 同机接入（工具和豆芽在同一台机器）无需开启此开关，直接用 `127.0.0.1` 即可。

### 2. 设置 API Key（强烈建议）
- 在 "服务 API KEY" 区块开启 **"启用 API Key 验证"**
- 在 API Key 输入框设置一个密钥（自定义字符串即可）
- 重启豆芽使 Key 生效

> 不设置 API Key 也可使用，但任何能访问该端口的设备都能调用你的模型，存在安全风险。

### 3. 加载模型
- 在豆芽主界面加载一个模型（如 Qwen3、Gemma 等）
- 外部工具调用前，模型必须处于已加载状态
- 也可通过 API 主动加载：`POST http://<host>:8080/models/load`，body 为 `{"name":"<模型文件名>"}`

---

## 二、通用接入信息

所有外部工具接入都需要以下三个参数：

| 参数 | 值 | 说明 |
|------|-----|------|
| **Base URL** | `http://<豆芽机器IP>:8080` | 同机用 `http://127.0.0.1:8080`；部分工具要求末尾加 `/v1` |
| **API Key** | 在豆芽设置页设置的 Service API Key | 若未启用 API Key 验证，可填任意值或留空 |
| **Model** | `default` 或具体模型名 | `default` 表示当前已加载的模型；也可通过 `/v1/models` 查询可用模型名 |

### 查询当前可用模型
```bash
curl http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer <你的APIKey>"
```

### 快速验证服务是否可用
```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer <你的APIKey>" \
  -H "Content-Type: application/json" \
  -d '{"model":"default","messages":[{"role":"user","content":"你好"}]}'
```

---

## 三、Claude Code 接入

Claude Code 是 Anthropic 官方的命令行编程助手。它默认使用 Anthropic Messages API，但也支持通过 OpenAI 兼容 provider 接入第三方服务。

### 方式一：通过 OpenAI 兼容 provider 配置

在 Claude Code 的配置文件（`~/.claude/settings.json` 或项目级 `.claude/settings.json`）中添加：

```json
{
  "providers": {
    "douya": {
      "type": "openai",
      "baseURL": "http://127.0.0.1:8080/v1",
      "apiKey": "<你的APIKey>",
      "models": ["default"]
    }
  }
}
```

启动时指定 provider：
```bash
claude --provider douya --model default
```

### 方式二：环境变量方式

```bash
# Windows PowerShell
$env:ANTHROPIC_BASE_URL = "http://127.0.0.1:8080"
$env:ANTHROPIC_API_KEY = "<你的APIKey>"
claude

# Linux/macOS
export ANTHROPIC_BASE_URL=http://127.0.0.1:8080
export ANTHROPIC_API_KEY=<你的APIKey>
claude
```

> **注意**：方式二使用 Anthropic Messages API 协议。llama-server 对 Anthropic 协议的兼容性取决于版本，若遇到协议不兼容错误，请优先使用方式一（OpenAI 兼容 provider）。

---

## 四、Codex (OpenAI CLI) 接入

Codex 是 OpenAI 官方的命令行编程工具，原生支持自定义 OpenAI 兼容端点。

### 环境变量配置

```bash
# Windows PowerShell
$env:OPENAI_BASE_URL = "http://127.0.0.1:8080/v1"
$env:OPENAI_API_KEY = "<你的APIKey>"
codex

# Linux/macOS
export OPENAI_BASE_URL=http://127.0.0.1:8080/v1
export OPENAI_API_KEY=<你的APIKey>
codex
```

### 指定模型

```bash
codex --model default
```

或通过配置文件 `~/.codex/config.json`：
```json
{
  "model": "default",
  "apiBaseUrl": "http://127.0.0.1:8080/v1",
  "apiKey": "<你的APIKey>"
}
```

---

## 五、OpenCoder / Open Claw 接入

OpenCoder、Open Claw 等开源编程助手通常在设置界面提供 OpenAI 兼容 endpoint 配置。

### 配置步骤

1. 打开工具的设置/偏好页面
2. 找到 "模型" 或 "Provider" 配置项
3. 选择 "OpenAI 兼容" 或 "Custom OpenAI Endpoint"
4. 填入以下信息：
   - **API Base URL**：`http://127.0.0.1:8080/v1`（跨机器用 `http://<豆芽机器IP>:8080/v1`）
   - **API Key**：在豆芽设置的 Service API Key
   - **Model Name**：`default` 或具体模型名
5. 保存配置并测试连接

> 不同工具的配置项名称可能略有差异，核心是找到"自定义 OpenAI 端点"或"OpenAI Compatible"选项。

---

## 六、通用 OpenAI 兼容客户端

任何支持自定义 OpenAI Base URL 的客户端均可接入豆芽，包括但不限于：

| 客户端 | 配置位置 |
|--------|----------|
| **OpenWebUI** | 设置 → 连接 → OpenAI API Base URL |
| **Cherry Studio** | 设置 → 模型服务 → 添加自定义 OpenAI Provider |
| **ChatBox** | 设置 → AI Provider → OpenAI API → 自定义域名 |
| **LobeChat** | 设置 → 语言模型 → OpenAI → 代理地址 |
| **AnythingLLM** | 设置 → LLM Provider → Generic OpenAI |

### 通用配置三要素
1. **Base URL**：`http://<豆芽机器IP>:8080/v1`（注意末尾的 `/v1`）
2. **API Key**：豆芽设置的 Service API Key
3. **Model**：`default` 或从 `/v1/models` 查询

---

## 七、可用 API 端点清单

豆芽（通过 llama-server）提供以下 OpenAI 兼容端点：

| 端点 | 方法 | 用途 |
|------|------|------|
| `/v1/chat/completions` | POST | 聊天补全（支持流式和非流式） |
| `/v1/completions` | POST | 文本补全（旧版接口） |
| `/v1/embeddings` | POST | 生成嵌入向量（用于 RAG/语义搜索） |
| `/v1/models` | GET | 列出可用模型 |
| `/v1/models/{name}` | GET | 获取指定模型信息 |
| `/models/load` | POST | 加载指定模型 |
| `/models/unload` | POST | 卸载指定模型 |
| `/props` | GET | 获取服务器属性 |
| `/health` | GET | 健康检查 |

---

## 九、上下文窗口配置（重要）

豆芽对**外部智能体的请求不做任何拦截或自动裁剪**（智能体直连 llama-server），上下文超出时会返回错误（`exceed_context_size_error`）。因此，请把各智能体的 `context-window`（上下文窗口）配置为豆芽的上下文大小（设置 → 上下文大小），让智能体自行管理历史裁剪。

### 查询豆芽当前的上下文大小

```bash
curl http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer <你的APIKey>"
```

返回的 `context_length` / `ctx` 字段即为当前上下文窗口大小（默认 8192，可在豆芽设置中调整）。

### 各智能体的上下文窗口配置

| 工具 | 配置方式 | 示例 |
|------|----------|------|
| **Claude Code** | `~/.claude/settings.json` 的 provider 配置，或环境变量 | `"env": {"CLAUDE_CODE_CONTEXT_WINDOW": "8192"}`（部分版本支持） |
| **Codex** | `~/.codex/config.json` | `{"model_context_window": 8192}` |
| **opencode** | `opencode.json` 的 model 配置 | `{"model": {"context_window": 8192}}` |
| **OpenCoder / Open Claw** | 模型设置 → 上下文窗口 | 填入与豆芽一致的数值 |

> 具体字段名以各工具当前版本的文档为准。核心原则：**把窗口设置为豆芽的上下文大小（或略小，如 8192→8000），留出余量给工具调用与生成。**

### 相关建议

- **溢出报错排查**：若智能体频繁报 `context size exceeded`，优先检查其 `context-window` 是否大于豆芽的上下文大小。
- **context-shift 兜底**：豆芽「KV 缓存 → 上下文移位」开关开启后，llama-server 对多轮生成的 KV 满溢有滑窗兜底（不适用于单次超长请求）。建议保持开启。

---

## 十、安全提示

1. **仅 HTTP 协议**：llama-server 不提供 HTTPS，API Key 在网络中明文传输。建议仅在可信局域网使用。
2. **务必设置 API Key**：开启局域网暴露后，任何能访问该端口的设备都可调用你的模型。设置 API Key 可防止未授权访问。
3. **跨公网使用需加反向代理**：如需从公网访问，请在豆芽前置 Nginx/Caddy 等反向代理，配置 HTTPS 和访问控制。
4. **防火墙配置**：开启暴露后，需在 Windows 防火墙中放行 8080 端口（或自定义端口）。
5. **资源占用**：外部工具的并发请求会占用 GPU 显存和算力，可能影响豆芽自身的对话体验。

---

## 十一、故障排查

### 连接超时 / 无法连接
- 检查豆芽是否正在运行，llama-server 是否已启动
- 检查 `expose_server` 是否已开启（跨机器访问时）
- 检查 Windows 防火墙是否放行对应端口
- 检查 IP 地址是否正确（同机用 `127.0.0.1`，跨机器用豆芽机器的局域网 IP）
- 用 `curl http://<host>:8080/health` 测试连通性

### 401 Unauthorized / 认证失败
- 检查 API Key 是否与豆芽设置页中设置的完全一致
- 检查 `server_api_key_enabled` 是否已开启
- 确认请求头格式为 `Authorization: Bearer <你的APIKey>`
- 注意：修改 API Key 后需重启豆芽服务才生效

### 404 Model Not Found / 模型未加载
- 先在豆芽 GUI 中加载一个模型
- 或通过 API 加载：`curl -X POST http://<host>:8080/models/load -H "Content-Type: application/json" -d '{"name":"<模型文件名>"}'`
- 使用 `default` 作为 model 名表示当前已加载的模型
- 通过 `GET /v1/models` 查询当前可用模型

### 响应缓慢 / 超时
- 检查模型大小是否超出 GPU 显存（导致 CPU 慢速推理）
- 减小 `max_tokens` 参数
- 关闭其他占用 GPU 的程序
- 检查是否同时有多个客户端在调用

### 流式响应中断
- 部分客户端默认超时较短，需调大请求超时时间
- llama-server 的单次生成超时为 300 秒（豆芽默认配置）

---

## 十、获取帮助

如遇本文档未覆盖的问题，可通过以下方式排查：
1. 查看豆芽应用日志（通常在应用目录下的 log 文件）
2. 查看 llama-server 控制台输出
3. 用 `curl` 直接测试 API 端点，排除客户端配置问题
4. 通过 `GET /v1/models` 和 `GET /props` 确认服务和模型状态
