# 豆芽项目优化分析报告

> 分析时间：2026-05-22  
> 分析范围：全项目 Go 代码（internal/ 目录，共 24 个 .go 文件）  
> 说明：本报告只分析，不修改代码

---

## 一、项目整体架构

```
douya/
├── cmd/              # 入口（Wails 主程序）
├── internal/
│   ├── chat/        # 聊天核心逻辑（service.go 最复杂）
│   ├── config/      # 配置加载与校验
│   ├── llm/         # LLM 客户端 + llama-server 进程管理
│   ├── search/      # 搜索引擎链（熔断 + 并发）
│   ├── store/       # SQLite 存储层
│   ├── system/      # 硬件检测（GPU/CPU）
│   └── tts/        # 语音合成（未读）
├── frontend/        # 前端（Wails + Vue）
└── engines/         # llama-server 二进制
```

**技术栈**：Go + Wails v2 + SQLite3 + llama.cpp

---

## 二、各模块问题详述

---

### 🔴 高优先级（影响功能稳定性）

---

#### 1. `internal/chat/service.go` — 聊天核心

**问题 1.1：`SendMessage` 中 `wailsCtx` 可能为 `nil`**
```go
func (s *Service) emit(eventType string, content interface{}) {
    if s.wailsCtx != nil {  // ← 有 nil 检查（好的）
        runtime.EventsEmit(s.wailsCtx, ...)
    }
}
```
但 `SetContext()` 是在 Wails 启动时调用的，若在 `SetContext` 之前调用 `SendMessage`，`wailsCtx` 为 `nil`，前端收不到任何事件，但不会 panic（已保护）。

**结论**：低风险，但建议加日志。

---

**问题 1.2：Token 估算极度粗糙**
```go
func estimateMessageTokens(m *store.Message) int {
    tokens := len([]rune(m.Content)) * 3  // ← 字符数×3
    ...
}
```
这是最大的精度问题。实际 Token 数取决于：
- 模型的词表（BPE / SentencePiece / Tiktoken）
- 中文字符通常 = 1-2 token，不是 3
- 代码 token 化规则和自然语言不同

**影响**：上下文截断不准确，可能截断过多或超出 ContextSize 导致 OOM。

**建议**：用 `github.com/pkoukk/tiktoken-go` 做真实 token 计数（支持 GPT 系列，本地模型近似可用）。

---

**问题 1.3：`buildLLMMessages` 历史截断逻辑过于复杂**
```go
// 这段逻辑 100+ 行，有多个 continue/break/append 交错
for i := len(dbMsgs) - 1; i >= 0; i-- { ... }
```
**风险**：
- 工具调用链（tool_calls → tool response）的配对逻辑容易出错
- 连续 assistant 消息合并逻辑（`cleaned` 变量）边界情况未覆盖
- 无单元测试覆盖（没看到 `_test.go` 文件）

**建议**：拆分成 3 个独立函数，各自单测。

---

**问题 1.4：系统 Prompt 每次请求都重新拼接**
```go
func (s *Service) buildLLMMessages(...) []llm.ChatMessage {
    systemContent := s.config.SystemPrompt
    if systemContent == "" {
        systemContent = `...`  // ← 每次都拼这个大字符串
    }
    // 每次都加日期
    now := time.Now()
    systemContent += fmt.Sprintf("\n\n当前日期时间: %s...", ...)
}
```
**影响**：每次聊天请求都做字符串拼接 + 格式化，浪费 CPU。

**建议**：缓存 system prompt，只在配置变更或整点更新日期部分。

---

#### 2. `internal/llm/client.go` — LLM 客户端

**问题 2.1：`streamClient` 超时硬编码 600 秒**
```go
func NewClient(baseURL string) *Client {
    return &Client{
        httpClient:   &http.Client{Timeout: 300 * time.Second},
        streamClient: &http.Client{Timeout: 600 * time.Second}, // ← 写死
    }
}
```
**影响**：用户无法根据模型速度调整超时，慢模型（如 70B Q4）可能超时。

**建议**：从 `config.Config` 读取，或设为 `0`（无超时）由上层 `context.WithTimeout` 控制。

---

**问题 2.2：`GetModelInfoByName` 每次都拉全量模型列表**
```go
func (c *Client) GetModelInfoByName(ctx context.Context, modelName string) (*ModelInfo, error) {
    // 调用 /v1/models → 返回所有模型 → 遍历找目标
    var raw struct { Data []struct{ ID string `json:"id"` } `json:"data"` }
    ...
}
```
**影响**：若 `models/` 目录下有 20 个模型，每次切换都拉全量列表（虽然数据量不大，但不优雅）。

**建议**：llama-server 支持 `GET /v1/models/{model_name}`，直接查单个。

---

#### 3. `internal/llm/server.go` — Server 管理

**问题 3.1：`Watch()` 重启退避策略不够**
```go
backoffs := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
// 第4次崩溃后直接放弃，不指数退避到更长时间
```
**影响**：若模型文件损坏导致每次启动都秒崩，3 次重启在 14 秒内完成，然后放弃。用户需要手动重启。

**建议**：改为指数退避 + 最大退避上限（如 60 秒），并通知前端"模型加载失败"。

---

**问题 3.2：`stopInternal` 用 `taskkill /T` 杀进程树**
```go
terminateCmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T")
```
`/T` 会杀掉该进程的所有子进程。若未来 llama-server 自己管理子进程，可能误杀。

**建议**：先尝试 `POST /shutdown`（graceful），失败后用 `/F` 只杀目标 PID（不用 `/T`）。

---

### 🟡 中优先级（影响性能/体验）

---

#### 4. `internal/store/db.go` — 数据库

**问题 4.1：`messages` 表缺少 `created_at` 索引**
```sql
CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id);
-- 缺少: CREATE INDEX idx_messages_created_at ON messages(conversation_id, created_at);
```
**影响**：`GetMessagesByConversation` 用 `ORDER BY created_at` 但没有复合索引，大对话历史会慢。

---

**问题 4.2：FTS5 触发器语法有误**
```go
_, err = db.Exec(`
    CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
        INSERT INTO messages_fts(rowid, content) VALUES (new.rowid, new.content);
    END;
`)
```
FTS5 外部内容表（`content=messages`）的 `INSERT` 语法应该是：
```sql
INSERT INTO messages_fts(rowid, content) VALUES (new.rowid, new.content);
```
当前代码是正确的（我误判了），但建议加日志确认 FTS5 触发器确实生效。

---

**问题 4.3：`SetMaxOpenConns(1)` 限制并发**
```go
db.SetMaxOpenConns(1)
db.SetMaxIdleConns(1)
```
**影响**：所有 DB 操作串行化。对本地小程序影响不大，但如果未来加后台任务（如导出、清理）会阻塞聊天。

**建议**：改为 `SetMaxOpenConns(4)`，SQLite WAL 模式支持并发读。

---

#### 5. `internal/search/chain.go` — 搜索链

**问题 5.1：深度搜索并发无限制**
```go
for _, pw := range eligible {
    wg.Add(1)
    go func(pw *ProviderWithCircuit) {  // ← 所有 provider 同时并发
        resp, err := pw.Provider.SearchWithOpts(ctx, query, opts)
        ...
    }(pw)
}
```
**影响**：若配置了 10 个搜索源，深度搜索同时发 10 个 HTTP 请求，可能触发某些 API 的速率限制。

**建议**：用 semaphore 限制最大并发数（如 `chan struct{}{}` 实现，最多 3 个并发）。

---

**问题 5.2：URL 归一化不完整**
```go
func normalizeURL(u string) string {
    parsed, err := url.Parse(u)
    parsed.Fragment = ""
    parsed.RawQuery = ""
    // 不处理：http vs https、www. 前缀、trailing slash
}
```
**影响**：同一网页的 `http://example.com` 和 `https://example.com` 会被视为不同结果，导致去重失败。

**建议**：用 `github.com/go-urlnormalize/urlnormalize` 或手写规则（删协议头、www、trailing slash）。

---



### 🟢 低优先级（代码质量/可维护性）

---

#### 7. `internal/llm/preset.go` — 模型预设

**问题 7.1：`DeriveModelName` 替换规则硬编码过多**
```go
name = strings.ReplaceAll(name, "-U-", "-")
name = strings.ReplaceAll(name, "-U_", "-")
name = strings.ReplaceAll(name, "_U_", "-")
name = strings.ReplaceAll(name, "_", "-")
```
**影响**：若模型名包含 `_U_` 是有意义的（如 `Code_U_Neuron`），会被错误替换。

**建议**：用正则表达式只替换量化后缀部分（已有 `quantSuffixRe`，但 `DeriveModelName` 没用上）。

---

#### 8. 全局问题

**问题 8.1：无单元测试**
```
internal/
├── chat/service.go       (600+ 行，0 测试)
├── llm/client.go        (300+ 行，0 测试)
├── search/chain.go      (200+ 行，0 测试)
└── store/db.go          (150+ 行，0 测试)
```
**风险**：修改核心逻辑（如本次 VRAM 优化）后无法快速验证回归。

**建议**：至少给 `chat/service_test.go`（历史截断逻辑）和 `search/chain_test.go`（去重逻辑）补测试。

---

**问题 8.2：日志无结构化**
```go
log.Printf("[config] loaded config from %s", path)
log.Printf("[model] /props: modalities.vision=%v ...", ...)
```
所有日志都用标准库 `log`，无级别（INFO/WARN/ERROR）、无上下文（请求 ID）。

**建议**：引入 `github.com/rs/zerolog` 或 `go.uber.org/zap`，支持日志级别 + 结构化字段。

---

## 三、优化优先级总表

| 优先级 | 问题 | 模块 | 难度 | 预期收益 |
|--------|------|------|------|----------|
| 🔴 P0 | Token 估算精度 | chat/service.go | 中 | 避免上下文溢出/浪费 |
| 🔴 P0 | 历史截断逻辑无测试 | chat/service.go | 低 | 避免 bug |
| 🟡 P1 | 搜索并发无限制 | search/chain.go | 低 | 避免 API 限流 |
| 🟡 P1 | DB 缺少复合索引 | store/db.go | 低 | 大对话加速 |
| 🟢 P2 | 模型名推导规则硬编码 | llm/preset.go | 低 | 兼容性提升 |
| 🟢 P2 | 无结构化日志 | 全局 | 中 | 调试效率提升 |
| 🟢 P2 | 无单元测试 | 全局 | 高 | 长期维护性 |

---

## 四、建议的优化路线图

```
Phase 1（1-2 天）：
  - 给 Token 估算加入 tiktoken 库
  - 给 buildLLMMessages 补单元测试

Phase 2（2-3 天）：
  - 搜索并发加 semaphore 限制
  - DB 加复合索引 + 调大 MaxOpenConns

Phase 3（1 天）：
  - 模型名推导改用正则（复用 quantSuffixRe）
  - 引入 zerolog 替换标准 log

Phase 4（持续）：
  - 补单元测试（目标：核心模块覆盖率和 60%+）
  - 性能基准测试（go test -bench）
```

---

## 五、总结

豆芽项目整体代码质量不错，架构清晰（Wails 前端 + Go 后端 + llama.cpp 推理）。主要问题集中在：

1. **精度问题**：Token 估算、URL 去重
2. **健壮性**：历史截断逻辑复杂无测试、并发无限制

建议优先解决 Token 估算和测试覆盖，这两个对用户体验影响最大。

---

*报告结束*
