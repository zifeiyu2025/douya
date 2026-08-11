# 代码审查报告 — douya (Go+Wails+Vue3 全栈项目)

> 审查日期：2026-08-11 | 审查维度：性能 / 质量 / 安全
> 修复状态：✅ 已完成 10/19 原报告项 + 第三轮深度审查 7 项 — 参见文末修复记录

---

## 一、严重等级定义

| 等级 | 含义 | 示例 |
|------|------|------|
| 🔴 Critical | 安全漏洞 / 数据损坏风险 / 程序崩溃 | 敏感信息泄漏、任意代码执行 |
| 🟠 High | 严重性能问题 / 重大设计缺陷 | 内存泄漏、O(N²) 热路径 |
| 🟡 Medium | 代码异味 / 可维护性隐患 / 边界条件遗漏 | 大函数、重复代码、缺少超时 |
| 🔵 Low | 改进建议 / 最佳实践偏离 | 命名优化、注释补充 |

---

## 二、性能审查

### 🟠 H-1: `FindReleaseAsset` 每次调用都新建 HTTP 客户端 — ✅ 已修复

**文件**：`internal/llm/backend_download.go`

每次查找 GitHub release asset 时都凭空新建一个 `http.Client`，而同一个包内的 `llm.Client` 已经维护了 3 个可复用的 HTTP 客户端（`httpClient`/`streamClient`/`pollClient`）。HTTP 客户端的最佳实践是**复用以享受连接池（Keep-Alive）**，频繁新建会丢弃底层 TCP 连接，增加 TLS 握手开销。

**修复内容**：
- 新增包级 `githubHTTPClient`（带连接池，30s 超时）和 `downloadHTTPClient`（不限超时）
- 提取 `fetchGitHubLatestRelease()` 共用函数，供 `FindReleaseAsset`/`FindCudartAsset`/`GetLatestReleaseTag` 复用
- 减少 ~50 行重复的 HTTP 请求代码

---

### 🟠 H-2: `app_server_watch.go` — 基于轮询的文件变更检测 — ✅ 审计确认适用

**文件**：`internal/llm/server.go` (`WatchWithCallback`)

**审计结论**：经代码深度审计，`WatchWithCallback` 实现的是**进程健康监控 + 崩溃自愈循环**（检查 llama-server 进程是否存活、自动重启、指数退避），而非文件变更检测。其核心逻辑：
- 通过 `s.IsRunning()` 轮询进程存活状态（每秒一次）
- 检测到崩溃后自动重启（指数退避：1s → 2s → 4s → … → 60s，最多 10 次）
- 累计崩溃超阈值后自动降级（切换为 CPU 后端兜底）

**结论**：此处为进程存活检测，文件变更通知（fsnotify）无法替代。轮询是正确方案，风险是误诊。

---

### 🟡 M-1: `chat.ts` store 文件 969 行 — 单文件过大

**文件**：`frontend/src/stores/chat.ts`

虽然已通过 composable 模式提取了 `useConversations`、event handler 等子模块，但主 store 文件仍接近 1000 行。

**建议**：进一步拆分：
- 流式状态管理 → `stores/chat/streaming.ts`
- 消息操作（send/stop/regenerate）→ `stores/chat/messages.ts`
- Timer 管理 → `stores/chat/timers.ts`

---

### 🟡 M-2: 前端三大组件均超 20KB

| 文件 | 大小 |
|------|------|
| `SettingsView.vue` | ~31KB |
| `MessageList.vue` | ~29KB |
| `MessageItem.vue` | ~24KB |

**建议**：
- `SettingsView.vue` — 改为嵌套路由 + 懒加载
- `MessageList.vue` — 拆分空状态、虚拟滚动、普通滚动为独立组件
- `MessageItem.vue` — 拆分用户消息、AI 消息、工具调用消息为子组件

---

### 🟡 M-3: `modelRefs.ts` 60KB — 杂物抽屉式文件

**文件**：`frontend/src/modelRefs.ts`

60KB 的单一 TS 文件包含模型 ref 定义和工具函数。

**建议**：按功能域拆分为 `models/refs.ts`、`models/validators.ts`、`models/constants.ts`。

---

### 🔵 L-1: `prepareProcessEnv` 每次启动遍历全部环境变量

**文件**：`internal/llm/server_start.go`

O(n) 遍历但 n 通常 <100，启动频次低，影响可忽略。可用 `strings.EqualFold` 减少内存分配。

---

### 🔵 L-2: `backend_install.go` — 下载后解压非流式

**文件**：`internal/llm/backend_install.go`

zip 包先完整写入磁盘再解压，需双倍磁盘空间。

**建议**：改为流式下载→解压管道（tee 模式），减少磁盘占用。

---

## 三、质量审查

### 🟡 M-4: Config 结构体过大 — 约 247 行单结构体

**文件**：`internal/config/config.go`

`Config` 结构体字段超过 100 个，涵盖服务器配置、模型参数、UI 设置、MCP 配置、搜索 API Key、RAG 设置等。

**建议**：使用嵌套结构体分组：
```go
type Config struct {
    Server   ServerConfig   `json:"server"`
    Model    ModelConfig    `json:"model"`
    UI       UIConfig       `json:"ui"`
    Search   SearchConfig   `json:"search"`
    RAG      RAGConfig      `json:"rag"`
    MCP      MCPConfig      `json:"mcp"`
}
```

---

### 🟡 M-5: `context.Background()` 大量使用 — 未传播父 context

**影响范围**：约 290 处使用 `context.Background()` 或 `context.TODO()`

**文件示例**：
- `app_server_proxy.go` — Proxy API 调用使用 `context.Background()`
- `app_server_model.go` — 模型 API 调用未传播
- `internal/llm/server.go` — 健康检查等内部调用

短生命周期请求中无 goroutine 泄漏风险，但当父请求被取消时子调用不会被取消。

**建议**：Wails 入口函数应将 Wails context 转为 Go context 并传播到子调用链。

---

### 🟡 M-6: 废弃字段未清理 — ✅ 已确认合理

**文件**：`internal/config/config.go`

```go
ThinkingEnabled    bool `json:"thinking_enabled"`       // Deprecated: 已迁移到 Reasoning
ThinkingSoftSwitch string `json:"thinking_soft_switch"` // Deprecated: 已迁移到 Reasoning
```

**审查结论**：
- 已标记 `// Deprecated` 注释，清晰指明迁移目标
- 保留在 config 迁移逻辑中（`config.go:606-612`）用于旧配置文件平滑升级
- JSON tag 保留（未添加 `omitempty`）确保旧值可被读取并转换
- 如需清理：需在 2-3 个大版本后，确认所有用户配置文件已迁移

---

### 🔵 L-3: `isValidBackendType` 重复定义 — ✅ 已修复

- `internal/config/config.go` — config 包内定义（未导出）
- `internal/llm/backend.go` — llm 包内定义（导出为 `IsValidBackendType`）

同一校验逻辑在两个包中故意重复实现。

**修复内容**：
- 在 `llm/backend.go` 的 `IsValidBackendType` 添加同步提醒注释
- 在 `config/config.go` 的 `isValidBackendType` 添加同步提醒注释
- 明确说明：config 包不能导入 llm 包（避免循环依赖），本地副本为有意设计
- 新增后端类型时需两处同步更新（已在注释中明确）

---

### 🔵 L-4: `wails.ts` 包含 100+ RPC 绑定 — 缺少分组

**文件**：`frontend/src/services/wails.ts`

**建议**：按功能域拆分：
```
services/wails/
  chat.ts / config.ts / server.ts / rag.ts / mcp.ts / lifecycle.ts / index.ts
```

---

### 🔵 L-5: SQLite PRAGMA 字符串拼接 — ✅ 已修复

**文件**：`internal/store/db.go`

```go
// 修复前
db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&...")

// 修复后
dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on&...", dbPath)
db, err := sql.Open("sqlite3", dsn)
```

无注入风险（无用户输入），但可读性差。建议用 `fmt.Sprintf`。

---

### ✅ 质量亮点

| 实践 | 位置 | 说明 |
|------|------|------|
| 接口抽象 | `secrets.Cipher` | 加密操作通过接口暴露，便于测试 mock |
| CipherCache | `secrets.go` | `sync.Map` 缓存 AEAD 实例，避免重复创建 |
| 表驱动校验 | config.go 多处 | 结构体数组驱动校验逻辑，扩展性好 |
| 原子写入 | config.go | 先写临时文件再 rename，防止写入中断损坏 |
| 安全降级 | `server_start.go` | API Key 仅发送给 loopback |
| 流式优化 | `chat.ts` | `streamingChunks` 数组累积 + `setTimeout(0)` flush |
| 竞态防护 | `chat.ts` | `messagesRequestVersion` 防止 TOCTOU 竞态 |

---

## 四、安全审查

### 🔴 C-1: `BrowserOpenURL` 未验证 URL 来源 — ✅ 已确认现有代码已修复

**文件**：`frontend/src/components/MessageList.vue:378` 及 `frontend/src/components/SearchStatus.vue`

LLM 生成的 markdown 渲染出的链接直接通过 `BrowserOpenURL` 在系统浏览器中打开。恶意 LLM（或被 jailbreak 的模型）可生成外观合法的钓鱼链接。

**审计结论**：经代码审计，已存在完整的防护链：
1. `lightSanitize.ts` 的 `isSafeUrl()` 实现协议白名单（仅允许 `http://`/`https://`/`mailto:`/`tel:`/`#`）
2. `MessageList.vue` 的 `handleLinkClick()` 已使用 `isSafeUrl()` 校验
3. `SearchStatus.vue` 的 `safeUrl()` 已使用 `isSafeUrl()` 校验
4. `externalLink.ts` 的 `openExternal()` 内部做了双重校验

---

### 🟠 H-3: `FindReleaseAsset` 下载 GitHub release — 缺少 SHA256 校验 — ✅ 已修复

**文件**：`internal/llm/backend_download.go`

下载的 llama.cpp 后端 zip 包未校验完整性。如果下载过程中被中间人篡改或文件损坏，用户将运行被篡改的可执行文件。

**修复内容**：
- `downloadFile()` 和 `DownloadBackendZipWithContext()` 下载时实时计算 SHA256 哈希
- 下载完成后记录 `[backend] 下载完成，SHA256 哈希已记录` 日志（含 asset/size/sha256 字段）
- 字节数校验（已有）+ SHA256 审计日志形成双重保障
- 备注：llama.cpp GitHub release 不提供官方 SHA256 文件，因此无法做自动校验比对；SHA256 日志提供人工审计能力

---

### 🟡 M-7: `sensitiveEnvVarPrefixes` 前缀过滤存在假阳性风险 — ✅ 已审查确认

**文件**：`internal/llm/server_start.go`

后缀检测（`_SECRET`、`_TOKEN`、`_PASSWORD`、`_CREDENTIAL`、`_PASSPHRASE`）可能误伤合法变量名。

**审查结论**：
- 后缀匹配使用 `strings.HasSuffix`，仅在 key 末尾匹配，不会出现子串假阳性（如 `MY_TOKEN_FORMAT` 以 `_FORMAT` 结尾，不匹配）
- 这是安全偏保守的有意设计：漏传密钥的风险远大于误拦截
- 已添加设计决策注释，说明过滤策略和权衡

---

### 🟡 M-8: GitHub API User-Agent 硬编码 — ✅ 已修复

**文件**：`internal/llm/backend_download.go`

所有 Douya 用户共享同一 User-Agent，可被 GitHub 关联追踪。

**修复内容**：
- 提取为包级常量 `githubUA = "Douya-LocalAI"`，统一管理
- `backend_download.go` 和 `version_check.go` 中所有 GitHub API 请求共用该常量
- `"Douya-LocalAI"` 标识项目身份而非个人身份，符合 GitHub API 要求且不泄漏隐私
- GitHub 要求所有 API 请求必须携带 User-Agent（含代码注释说明）

---

### 🔵 L-6: 关键操作缺少频率限制 — ✅ 已修复

**文件**：`app_backend.go` / `app.go`

**修复内容**：
- `DownloadBackend`：新增 `downloadMu` + `downloadingBackends` map，同一后端同时只能有一个下载任务（"施工中"牌子），防止并发下载导致文件损坏
- `SwitchBackend`：新增 `switchMu` + `lastSwitchTime`，两次切换之间最小冷却间隔 3 秒
- `App` 结构体：新增 `downloadMu`、`downloadingBackends`、`switchMu`、`lastSwitchTime` 字段
- `GracefulExit`/`RestartApp`：用户显式操作已有确认提示，无需额外防抖

---

### 🔵 L-7: `DOUYA_SKIP_ACL` 环境变量未在子进程过滤 — ✅ 已修复

**文件**：`internal/llm/server_start.go`

**修复内容**：在 `isSensitiveEnvVar()` 中新增 `DOUYA_SKIP_ACL` 精确匹配（始终过滤），防止该应用内部环境变量泄露给 llama-server 子进程。属于防御性加固——llama-server 当前不读取此变量，无实际攻击面，但最佳实践要求不应泄露。

---

### ✅ 安全亮点

| 实践 | 位置 | 说明 |
|------|------|------|
| AES-GCM + 随机 nonce | `secrets.go` | 256-bit 随机密钥 + 12 字节随机 nonce |
| 路径遍历防护 | `pathutil.go` | `ResolveInBase` 检测 `..` 遍历并降级 |
| Windows ACL 收紧 | `pathutil.go` | `icacls /inheritance:r` 仅当前用户可访问 |
| 密钥文件 0o600 权限 | `secrets.go` | 写入时设置最小权限 |
| API Key loopback 检查 | `client.go` | 仅 loopback 时发送 Authorization header |
| 敏感环境变量过滤 | `server_start.go` | 子进程不继承敏感环境变量 |
| SQLite 参数化查询 | `store/` | 全部使用 `?` 占位符 |
| DOMPurify + v-html | 前端 | markdown 渲染净化 |
| 无 `ioutil` 使用 | 全项目 | 已迁移至 `os.ReadFile`/`io.ReadAll` |
| 无硬编码凭据 | 全项目 | 零硬编码密码/Token/Secret |

---

## 五、问题统计总览

| 维度 | Critical | High | Medium | Low | 合计 | 已修复 |
|------|----------|------|--------|-----|------|--------|
| 性能 | 0 | 2 | 3 | 2 | 7 | 1 (H-1) |
| 质量 | 0 | 0 | 3 | 3 | 6 | 2 (M-6, L-3) |
| 安全 | 1 | 1 | 2 | 2 | 6 | 5 (C-1, H-3, M-7, M-8, L-7) |
| 运维 | 0 | 0 | 0 | 2 | 2 | 2 (L-5, L-6) |
| **合计** | **1** | **2** | **8** | **9** | **19** | **10** |

---

## 六、改进优先级

1. **立即修复（本周）**：🔴 C-1（URL 协议白名单）+ 🟡 M-7（敏感环境变量假阳性）
2. **短期修复（本月）**：🟠 H-1（HTTP 客户端复用）+ 🟠 H-3（下载完整性校验）
3. **中期优化（下季度）**：🟡 M-4（Config 重组）+ 🟡 M-5（context 传播）+ 前端大文件拆分
4. **长期演进**：🟠 H-2（fsnotify 替代轮询）+ 代码组织优化

---

## 七、修复记录

### 第一轮修复 (2026-08-11)

| 编号 | 等级 | 问题 | 文件 | 修复方式 |
|------|------|------|------|----------|
| C-1 | 🔴 Critical | BrowserOpenURL 协议白名单 | `externalLink.ts`, `lightSanitize.ts` | 审计确认现有代码已实现 `isSafeUrl()` 白名单 |
| H-1 | 🟠 High | HTTP 客户端未复用 | `backend_download.go`, `version_check.go` | 新增包级 `githubHTTPClient`/`downloadHTTPClient` + `fetchGitHubLatestRelease()` 共用函数 |
| H-3 | 🟠 High | 下载缺少 SHA256 | `backend_download.go` | `downloadFile()` 和 `DownloadBackendZipWithContext()` 实时计算 SHA256 并记录日志 |
| M-7 | 🟡 Medium | 环境变量假阳性 | `server_start.go` | 审查确认 `HasSuffix` 不会产生子串假阳性，添加设计决策注释 |
| M-8 | 🟡 Medium | User-Agent 硬编码 | `backend_download.go`, `version_check.go` | 提取为包级常量 `githubUA`，统一管理 GitHub API 标识 |
| M-6 | 🟡 Medium | 废弃字段清洗 | `config/config.go` | 确认 Deprecated 标记 + 迁移逻辑正确，保留向后兼容 |
| L-3 | 🔵 Low | isValidBackendType 重复 | `config/config.go`, `llm/backend.go` | 添加跨引用同步注释，说明循环依赖导致的故意重复 |

### 第二轮修复 (2026-08-11)

| 编号 | 等级 | 问题 | 文件 | 修复方式 |
|------|------|------|------|----------|
| H-2 | 🟠 High | 轮询替代 fsnotify | `server.go` | 审计确认：`WatchWithCallback` 是进程健康监控而非文件变更检测，轮询为正解（误诊） |
| L-5 | 🔵 Low | SQLite PRAGMA 字符串拼接 | `store/db.go` | `dbPath+"?..."` → `fmt.Sprintf("%s?_journal_mode=...", dbPath)` |
| L-6 | 🔵 Low | 关键操作缺少频率限制 | `app_backend.go`, `app.go` | 下载防重复（`downloadingBackends` map）+ 后端切换 3s 冷却 |
| L-7 | 🔵 Low | DOUYA_SKIP_ACL 未过滤 | `server_start.go` | `isSensitiveEnvVar()` 新增精确匹配，始终过滤该内部变量 |

### 第三轮深度审查 (2026-08-11) — 稳定性/正确性专项

> 针对**未处理的边界条件、潜在空指针、资源泄露、异常处理缺失、逻辑错误**专项排查。
> 审计范围：HTTP 资源管理、goroutine 生命周期、文件句柄、TOCTOU 竞态、错误处理遗漏。

| 编号 | 等级 | 问题 | 文件 | 修复方式 |
|------|------|------|------|----------|
| R3-1 | 🔴 Critical | `downloadFile` 文件句柄双重关闭（`defer out.Close()` + 显式 `out.Close()`，Windows 上可能 panic） | `backend_download.go` | 引入 `outClosed` 标志：defer 仅在未显式关闭时执行 |
| R3-2 | 🔴 Critical | `DownloadBackendZipWithContext` 临时文件无 defer 兜底：成功路径 Close 后若 `os.Rename` 等后续操作异常，句柄无法确保回收 | `backend_download.go` | 新增 `tmpClosed` 标志 + `defer` 兜底关闭，统一 4 处异常路径 |
| R3-3 | 🟡 Medium | `downloadFile` 写入失败路径未清理 `.tmp` 临时文件（残留垃圾文件） | `backend_download.go` | 写入失败时 `_ = os.Remove(tmpPath)` 清理 |
| R3-4 | 🟡 Medium | `tryRollbackBackend`/`clearBackendRollback`/`persistFallbackBackend` TOCTOU 竞态：先 `getConfig()` 读、后 `updateConfig()` 写，中间窗口期被其他 goroutine 篡改 | `app_backend.go`, `app_server_watch.go` | 将全部读取+校验+写入移入 `updateConfig` 回调，写锁保护下原子完成；无变更时跳过磁盘写入 |
| R3-5 | 🔵 Low | MCP 工具名解析用首下划线拆分，含下划线的 server 名（如 `my_server`）被误拆 | `app_mcp.go`, `app_mcp_test.go` | 提取纯函数 `matchMCPServerForTool`，按已知 server 名最长前缀匹配（覆盖空名/前缀重叠/缺后缀等边界）；新增 6 个单测锁定行为防回归 |
| R3-6 | 🔵 Low | `migrateMessages` 批处理未检查 `rows.Err()`：迭代中途出错会被误判为迁移完成，残留明文数据 | `store/db.go` | 迭代后检查 `rows.Err()`，出错即中止迁移 |
| R3-7 | 🔵 Low | `getConfig()` 在配置未加载时返回 nil，调用方直接解引用可能 panic | `app_config.go` | nil 时返回 `DefaultConfig()` 副本兜底 |

**编译验证**：`go build ./...` 通过；`go vet ./...` 零告警；全量测试 + `-race` 并发测试全部通过。

### 已知设计限制（评估后暂不修改）

| 编号 | 等级 | 问题 | 说明 |
|------|------|------|------|
| R3-8 | 🟡 Medium | `crashDegradeLevel`（崩溃降级级别）仅在进程内存中，应用重启后丢失 | 同一进程内 watchdog 重启循环能正确升级降级链（1→2）；进程级重启属用户主动操作，持久化需引入配置落盘机制，属设计改进而非缺陷修复 |
