# 豆芽 Douya 全面代码审计报告

> 审计范围：代码逻辑 · 代码质量 · 代码卫生 · 使用体验（对标 LM Studio）
> 审计方式：全库静态审查（Go 后端 + Vue 前端），关键结论均附源码行号，可逐一复核
> 审计日期：2026-08-23
> 配套文档：《豆芽 Douya 全面代码审计 + 后端加固 + 前端彻底重构计划》（`.trae/documents/douya-audit-lmstudio-ux-improvement-plan.md`）

---

## 一、执行摘要

豆芽的**工程纪律高于同类开源项目平均水平**：Go 侧并发防护成熟、拥有统一错误框架与结构化日志；前端类型纪律严明（全库仅 15 处 `any`）、store 层具备竞态防护与编译期断言、XSS 双防线。

债务集中在四处：

1. **Go 侧 4 处高危缺陷**（数据竞争嫌疑 ×1、错误吞没 ×3 类）——均为小改动可修复，但发生在核心链路（模型进程管理 / 流式对话 / 历史消息渲染），对正在上架微软商店的应用而言，崩溃 = 差评，优先级最高；
2. **前端视图层体量失控**——14 个超 500 行的 God 组件、30+ 文件平铺、45 字段巨型 context 含类型逃逸，这正是本轮"彻底重构"的标的；
3. **UX 对标 LM Studio 的差距**——GGUF 元数据（参数量/量化/上下文长度）后端早已解析完毕却无任何 UI 渲染，聊天参数面板、模型详情卡片、消息编辑、会话日期分组缺失。好消息是：后端地基完备，绝大多数功能属于"接线 + 展示"工作；
4. **两处结构性问题**（双巨型镜像结构体、switch 编排层业务泄漏）——影响长期可维护性，本轮仅记入路线图不动代码（微软商店上架期的稳定性约束使然）。

### 总评分数卡（满分 10）

| 维度 | 得分 | 一句话评价 |
|---|:---:|---|
| Go 并发安全 | **8.0** | 防护体系成熟，但 `CloseJob` 存在一处漏网之鱼 |
| Go 错误处理 | **6.5** | apperror 框架优秀，但序列化/反序列化路径吞错成风 |
| Go 架构结构 | **6.0** | 分层清晰，但 Config 双镜像结构与 switch 层泄漏拖累长期演化 |
| 前端类型纪律 | **8.5** | any 仅 15 处、编译期断言开箱即用，同类项目罕见 |
| 前端组件结构 | **4.5** | God 组件泛滥、context 类型逃逸、composable 不可重入 |
| 前端性能意识 | **7.5** | 流式分块 flush / shallowReactive / HEIC 懒加载是好实践；虚拟滚动未转正 |
| 测试覆盖 | **6.0** | Go 侧覆盖厚实；前端 store 测试好、组件渲染测试近乎为零 |
| 样式卫生 | **5.0** | 全局 style.css 近 2000 行 + scoped/非 scoped 混用 + 62 处内联样式 |
| UX 开箱即用 | **6.5** | 底盘扎实（自动显卡判断/双源下载器/终端控制台），LM Studio 式细节缺位 |
| 文档与工程纪律 | **9.0** | README 技术栈版本锁定表、CI 三件套、pre-commit、renovate 齐备 |

**综合：7.0 / 10 —— 一个底子很好的项目，正处在"能跑"到"好用"的临界点上。**

---

## 二、详细发现

### 2.1 Go 后端

#### 🔴 H-1｜`CloseJob()` 无锁读取受保护字段 —— 数据竞争嫌疑

- **位置**：[internal/llm/server.go:1049-1053](file:///d:/MyGoWorkspace/douya/internal/llm/server.go#L1049-L1053)
- **现象**：

```go
func (s *Server) CloseJob() {
	if s.job != nil {
		s.job.Close()
	}
}
```

`s.job` 是受 `s.mu` 保护的共享字段（写入方在启动/停止流程中持锁赋值），而此处**既不加锁读、也不加锁调 Close**。对照同文件相邻方法即可见问题——[SetStatus (1037)](file:///d:/MyGoWorkspace/douya/internal/llm/server.go#L1037-L1041)、[Ctx (1043)](file:///d:/MyGoWorkspace/douya/internal/llm/server.go#L1043-L1047) 全都规规矩矩持锁。
- **风险**：若用户在推理进行中关闭窗口（触发 `CloseJob`）同时后台 goroutine 正在切换 job，构成典型 data race。轻则 `-race` 报警，重则读到半初始化对象导致崩溃。**对商店版应用而言这是最需要消除的一类缺陷。**
- **修复建议**：函数体内以 `s.mu.Lock()` 包裹（约 3 行改动），与既有锁约定注释对齐。✅ 已列入本轮 B-1 任务。

#### 🔴 H-2｜流式路径 `json.Marshal` 吞错

- **位置**：[internal/chat/service_stream.go:779](file:///d:/MyGoWorkspace/douya/internal/chat/service_stream.go#L779)、[783](file:///d:/MyGoWorkspace/douya/internal/chat/service_stream.go#L783)
- **现象**：

```go
if len(params.Images) > 0 {
    imgJSON, _ := json.Marshal(params.Images)   // 错误被 _ 吞掉
    userMsg.Images = string(imgJSON)            // 失败时落库空串
}
```

用户消息携带图片/附件时，序列化一旦失败，落库的是**静默损坏的数据**（空串），且无任何日志。未来重开会话时该消息"图片凭空消失"，用户无从排查，开发者也无迹可循。
- **风险**：中等概率低危害，但破坏"数据可信"底线；排障成本高。
- **修复建议**：出错时 `log.Warn().Err(err)` 留痕并跳过该字段。✅ 已列入本轮 B-2 任务。

#### 🔴 H-3｜历史消息反序列化失败静默降级（3 处）

- **位置一**：[internal/chat/service.go:294-306](file:///d:/MyGoWorkspace/douya/internal/chat/service.go#L294-L306) —— `storeMsgToChat` 中附件 JSON 解析失败时整个附件列表无声丢弃；
- **位置二**：[internal/chat/service_messages.go:733](file:///d:/MyGoWorkspace/douya/internal/chat/service_messages.go#L733) —— `buildHistoryUserMessage` 中历史附件解析失败时**静默降级为纯文本消息**（多模态上下文丢失而模型与用户均不知情）;
- **位置三**：[internal/chat/service_messages.go:827-838](file:///d:/MyGoWorkspace/douya/internal/chat/service_messages.go#L827-L838) —— assistant 消息的 ToolCalls 解析失败时整条工具调用记录被跳过，可能造成对话上下文不连贯。
- **共性根因**：`if err := json.Unmarshal(...); err == nil { ... }` 这种"成功才处理、失败装没事"的写法在三处复制蔓延。数据库中脏数据的真实规模因此完全不可观测。
- **修复建议**：补 `log.Warn()`（沿用同文件 zerolog 既有规范）。✅ 已列入本轮 B-3 任务。

#### 🟡 M-1｜测试导出后门绕锁裸写共享字段

- **位置**：[internal/chat/service.go:436-439](file:///d:/MyGoWorkspace/douya/internal/chat/service.go#L436-L439)

```go
func SetCurrentCancel(s *Service, fn context.CancelFunc) { s.currentCancel = fn } // Exported for testing
```

`s.currentCancel` 在生产代码中全部持锁读写，此测试辅助函数却在无锁状态裸写。它虽只被测试调用，但导出后任何包都可调用，等于在并发防护网上开了个洞。
- **修复建议**：函数体内加锁后再写，签名不变。✅ 已列入本轮 B-4 任务。

#### 🟠 R-1｜结构性问题（本轮仅入路线图）

| 问题 | 证据 | 说明 |
|---|---|---|
| Config ↔ ServerConfig 双巨型镜像结构体 | internal/config/config.go 等，两侧各约 228 个字段 | 同一份配置存两个几乎逐字段相同的结构体，新增配置项要同步改 6+ 处，极易漏改 |
| `app_server_switch.go` 业务逻辑泄漏到编排层 | 单文件 953 行 | 模型切换的状态机/回退/预热逻辑本应下沉至 internal/modelruntime，编排层只剩薄壳 |
| 错误包装双风格并存 | `%w` 与 `apperror.Wrap` 混用 | `errors.Is` 判定链路依赖调用者自觉，建议 lint 规则收敛 |

### 2.2 前端

#### 🔴 F-1｜God 组件泛滥，视图层体量失控

14 个组件超过 500 行，最重的几个：

| 组件 | 体量 |
|---|---|
| KnowledgeView.vue | 941 行 |
| MessageItem.vue | 923 行 |
| MessageList.vue | 926 行 |
| SettingsView.vue（script 部分） | 805 行 |
| AppHeader.vue（script 部分） | 478 行 |

30+ 组件平铺在 `components/` 根目录，无域划分。后果：改一处聊天交互要在近千行文件里翻找；新人（包括未来的你）无法建立心智地图；Naive UI 组件、业务逻辑、协议细节纠缠在同一 `<script>` 内。

#### 🔴 F-2｜45 字段巨型 context + 类型逃逸

- **位置**：[frontend/src/components/settings/settingsContext.ts:74](file:///d:/MyGoWorkspace/douya/frontend/src/components/settings/settingsContext.ts#L74)

```ts
// Store（pinia store 类型过于复杂，保留 any；实际访问的属性均有类型保障）
settingsStore: any
```

provide/inject 的 context 膨胀到 45 个字段，且核心 store 以 `any` 逃逸——TypeScript 对 settings 域的 14 个面板组件**实际处于失防状态**。注释自证了这是无奈妥协而非设计选择。

#### 🟡 F-3｜composable 不可重入

`useModelSwitch` 内部直接注册 Wails 事件监听且无单例保护——第二次调用就会重复注册。导致消费方 AppHeader 不敢复用它，手工复制了一份派生逻辑（478 行 script 的直接成因之一）。**这是"抽象失效→被迫复制→体量爆炸"链条的典型案例。**

#### 🟡 F-4｜组件渲染测试近乎为零

`__tests__/` 下 14 个测试文件覆盖 store/utils 层尚可，但**没有一个 God 组件的挂载渲染测试**。更糟的是 tsconfig.json 将 `*.test.ts` 排除出 typecheck——测试代码本身游离于类型检查之外，测试质量无从保证。

#### 🟡 F-5｜样式混杂

全局 style.css 约 1983 行 + 各组件 scoped/非 scoped 混用 + 62 处内联 style。科幻风视觉规格（色彩/渐变/毛玻璃参数）散落在无数魔法值里，主题一致性靠人肉维持——这也是"彻底重建 + 设计令牌固化"的直接动机。

#### 🟠 F-6｜协议包袱

stores/chat.ts 为兼容三种历史事件 payload 格式保留了双命名兼容分支。后端与前端同为本地代码、同版本发布，历史兼容层只会增加理解成本，应趁重建一并清理（后端格式唯一事实化）。

#### 性能现状（好坏参半）

- ✅ 好实践需保留推广：流式分块 flush（FLUSH_INTERVAL=0）、`shallowReactive` 降低响应式开销、StreamHandlerMap 编译期断言、HEIC 解码 Worker 化懒加载、XSS 消毒双防线、messagesRequestVersion TOCTOU 防护；
- ⚠️ 虚拟滚动（useVirtualScroll）已实现却只是实验性分支，长会话默认走普通渲染——性能利器没有上岗。

### 2.3 UX 对标 LM Studio 差距清单

| 能力 | LM Studio | 豆芽现状 | 差距定性 |
|---|---|---|---|
| 显卡判断/后端推荐 | 自动检测 GPU → 推荐 backend | ✅ 已有（多厂商检测 + VRAM 估算） | 基本持平，需验证精度 |
| 模型下载器 | 双源(HF/Mirror) + 进度 | ✅ 已有双源下载器 | 持平 |
| 本地服务端控制台 | 内置 server + 日志 | ✅ ConPTY 终端 + 指标 | 持平偏强 |
| **模型详情展示** | 参数量/量化/上下文一目了然 | ❌ GGUF 元数据后端已解析并流到 store，**无组件渲染**；量化靠文件名正则猜 | 纯接线工作 |
| **聊天内参数面板** | 会话侧栏实时调参 | ❌ 只能去设置页深处找，且请求级生效语义不明 | 新建 ParamsPanel |
| **消息编辑** | 编辑历史消息重生成 | ❌ 无 | 需补存储层 Update + 复用截断重发机制 |
| **会话日期分组** | 今天/昨天/更早 | ❌ 平铺列表 | 纯前端工作 |
| 资源实时监控 | VRAM/CPU 曲线 | ❌ 无 | 用户已明确不做 |

> 结论：豆芽的"底座能力"（显卡判断、下载器、服务端控制台、Token 计数）已达 LM Studio 水准；缺的是四件"看得见摸得着"的体验件，且后端地基完备，适合随前端重建一体化落地。

---

## 三、亮点清单（值得保持的资产）

**Go 侧**
1. 代际令牌（genToken）+ trackedGo goroutine 管理——后台任务不失控；
2. 多把细粒度锁替代单把粗锁，锁约定有注释体系；
3. apperror 统一错误框架（Kind 分类 + errors.Is 哨兵）；
4. zerolog 结构化日志带 `[tag]` 前缀约定；
5. nvidia-smi 调用带 HideWindow 防闪窗先例（vram.go）；
6. GGUF 元数据解析链路完整（NParams/SizeLabel/FileType/ContextLength）；
7. Sleep-Idle 引擎休眠机制省资源。

**前端侧**
1. 全库仅 15 处 any，types/ 独立目录；
2. 流式管线：分块 flush + shallowReactive + StreamHandlerMap 编译期断言；
3. XSS 双防线（markdown 消毒 + lightSanitize）；
4. messagesRequestVersion 防 TOCTOU 竞态。

**工程侧**
1. CI 三件套（lint/test/govulncheck）+ golangci + renovate + pre-commit + 版本一致性脚本。

---

## 四、后续路线图（状态跟踪）

> 本表随实施进度持续更新。✅ = 已落地；🔄 = 进行中/部分落地；⏳ = 未做（附决策）。

### 4.1 ⭐ 微软商店上架适配专节

> 背景：应用正在上架微软商店（MSIX 打包）。以下事项在商店版语境下重要性陡增，本轮仅评估不动代码。

1. **数据目录迁移评估** ✅ **已落地**：`internal/appdata` 包统一数据根为 `%LOCALAPPDATA%\Douya`（回退链 LOCALAPPDATA → APPDATA → exe 目录），`MigrateLegacyData` 提供旧目录（config.json + data/）一次性迁移，带幂等标记、失败降级不阻塞启动（含单元测试 `internal/appdata/appdata_test.go`）。MSIX 沙箱下安装目录只读不再影响数据读写。
2. **自更新与商店更新的冲突** ✅ **已落地**：按商店政策 10.1.5 移除应用内自更新（原 `app_update.go` 不再存在），更新统一由 Microsoft Store 接管（见 app_about.go 注释），双更新源互相破坏的灾难场景从根上消除。
3. **稳定性权重佐证** ✅：B-1 数据竞争修复（CloseJob 加锁）已随阶段 B 落地，`go test -race` 无告警。

### 4.2 Go 结构性重构（下一轮大版本候选）

1. **Config ↔ ServerConfig 合并** ⏳ **决策：不做结构体合并（ADR-2026-09）**。勘察确认 `ServerConfig`（internal/llm）是「用户配置子集 + 运行期派生值 + 环境路径」的启动 DTO，与持久化的 `Config` 生命周期不同，强行合并会把持久化与运行时启动耦合（反模式）。现状 `buildServerConfigFromFields`（app_server_config.go:161）已是 Config→ServerConfig 的**单一映射源**，`derive` 派生参数独立收敛于 `derivedServerParams`——「新增配置项要同步改 6+ 处」的问题已被单一函数 + 逐字段映射缓解。维持现状，不再追求字段级合并。
2. **switch 编排层下沉** ⏳ **决策：本轮不动（ADR-2026-09）**。`app_server_switch.go`（890 行）30 个函数全部强耦合 `App` 生命周期（`a.ctx`/`a.rootCtx`/`a.trackedGo`/`a.getClient()`/事件发射 helper），下沉至 internal/modelruntime 需要大规模接口反转，且商店上架期稳定性权重最高、无独立可回滚边界。**未来实施路径**：先把「纯计算」子集（`classifyWaitError`/`buildStackOverflowSuggestion`/`calculateLoadTimeout`/`getModelFileSize` 等）抽为无 App 依赖的纯函数迁入 internal/modelruntime 并附单测，再逐步迁状态机，最后编排层只剩薄壳。
3. **错误包装风格统一** ✅ **已落地**：全库统计 `apperror.Wrap`/`New` 517 处、`WrapNew` 158 处，裸 `fmt.Errorf %w` 仅剩 6 处且全部位于 tts 协议层（errors.Is 透传根因的合理场景）。`internal/chat/model_params.go` 两处面向用户的 `%w` 已统一为 `apperror.Wrap`/`New`；使用约定（业务层走 Wrap、库/协议层可 `%w`、禁止用户路径裸 `%w`）已固化到 `internal/apperror` 包文档。
4. **魔法数字常量化 + 日志语言统一** 🔄 **持续项**：零散魔法值仍散布于个别参数计算处（如 KeepSize=512 等已有注释说明），日志中英混排为长期收编项，随各模块改造顺手收敛，不设独立大改。

### 4.3 前端后续演进

1. **settings context 按域拆分** ✅ **已落地**：45 字段平铺 context 已重构为五个类型化域切片（core/appearance/aiChat/performance/apiService），API 类型由各 composable `ReturnType` 自动推导（settingsContext.ts:21 行）。
2. **组件渲染测试全覆盖** 🔄 **进行中**：已新增 `SessionSidebar`（标题过滤/全文聚合/定位跳转/空态/已删会话过滤，6 例）与设置章节搜索纯逻辑（`utils/sectionSearch`，13 例）；既有 MessageItem/ParamsPanel/ModelDetailCard 渲染测试持续扩充中。
3. **modelRefs.ts 迁移 JSON 资源** ✅ **已落地**：70 模型参数数据本体已迁至 `modelRefs.data.json`（约 90KB），`modelRefs.ts` 仅保留类型契约与统一导出，构建时由 codeSplitting 拆为独立分块（消费方导入路径零变化，`utils/modelRefs.test.ts` 34 例保持绿色）。
4. **Storybook 视觉回归基线** ⏳ **低优先**：ui/ 原子组件数量尚少，引入 Storybook 的维护成本高于当前收益，暂缓；以 tokens.css 设计令牌 + 组件渲染测试作为替代基线。

---

*本报告由全面静态审计产出；动态行为（竞态实测、性能剖析）随阶段 B/D 的验证命令（`go test ./... -race`、构建产物对比）持续补充实证。*
