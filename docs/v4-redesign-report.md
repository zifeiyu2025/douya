# 豆芽 · v4「深空苗圃」前端重设计交付报告

> **版本**：v4.0 ｜ **交付日期**：2026-08-25 ｜ **范围**：frontend 前端全部界面层
> **验证状态**：✅ 226 测试全绿 ｜ ✅ typecheck 零错误 ｜ ✅ 生产构建通过 ｜ ✅ ESLint 零警告

---

## 一、项目概述

### 1.1 重设计背景

豆芽是一款基于 Wails（Go 后端 + Vue 3 前端）的本地大模型桌面客户端。本次 v4 重设计对前端界面层进行全面换新，目标是对标 Raycast、Warp、Claude 等现代应用的视觉与交互水准。

### 1.2 核心设计决策

| 决策项 | 选定方案 |
|-------|---------|
| 视觉方向 | 深邃科技风（对标 Raycast / Warp） |
| 布局策略 | 彻底重新设计骨架（非换皮式微调） |
| 字体方案 | 内置现代品牌字体（离线可用，按需分包加载） |
| 背景图功能 | 完整保留，且新材质体系自动适配 |

### 1.3 设计语言：「深空苗圃」

- **主色板**：深蓝黑画布 × 芽绿荧光
  - 暗色主色 `#3DDC97`（芽绿荧光），亮色主色档 `#0e9f6e`
  - 暗色底 `#0A0F16`，亮色底 `#f5f8fa`
- **圆角阶梯**：小件 6px / 标准 10px / 大容器 12~14px / 输入舱专属 20px / 胶囊 999px
- **动效曲线**：全局统一弹性曲线 `--ease-spring`

---

## 二、设计系统架构

### 2.1 三层表面体系（背景图适配的核心）

所有界面表面归入三层语义，由 CSS 变量 alpha 通道驱动——**开启背景图时自动变通透，关闭时自动退化为实色**，无需两套样式：

| 层级 | 变量 | 用途 | 典型场景 |
|------|------|------|---------|
| veil 结构层 | `--surface-veil` | 页面打底 | 设置页/知识库容器 |
| panel 阅读层 | `--surface-panel` | 承载内容的面板 | 输入舱、菜单、模态框 |
| card 浮起层 | `--surface-card` | 浮起卡片 | 悬浮侧栏、状态胶囊 |

### 2.2 玻璃材质公式（六处悬浮件共享）

```
半透明面板底 + backdrop-filter: blur(20px) + 发丝描边 + 浮起阴影
```

覆盖部件：**悬浮侧栏、右上状态胶囊、侧栏 FAB 唤起钮、底部输入舱、下拉菜单、模态对话框**

### 2.3 字体体系

三套可变字体（variable fonts）内置打包，按字符集分包、按需加载：

- **Manrope** —— 拉丁字符主字体
- **Noto Sans SC** —— 中文字体
- **JetBrains Mono** —— 代码等宽字体

### 2.4 主题机制

- `useThemeOverrides.ts` 维护 LIGHT/DARK 双调色板（唯一颜色源），派生 Naive UI 全组件皮肤
- 体外弹窗组件（消息提示/确认框）经 `discrete.ts` Proxy 包装实现主题感知，暗色切换时自动重建实例
- 47 项 `themeTokens` 契约测试守护调色板与 tokens.css 的一致性

---

## 三、六阶段实施记录

### 阶段① 设计系统基建 ✅

**目标**：为后续所有视觉工作打下"唯一真相源"地基。

- 建立 [tokens.css](../frontend/src/styles/tokens.css) 令牌体系：颜色、圆角阶梯、动效曲线、消息流宽度（`--msg-max-width: 720px`）全部令牌化
- 接入三套品牌字体的分包加载链路（[main.ts](../frontend/src/main.ts)）
- 重写双主题调色板（[useThemeOverrides.ts](../frontend/src/composables/useThemeOverrides.ts)），Naive UI 组件全面接入三层表面变量
- 新增契约测试守护设计令牌（themeTokens.test.ts，47 项）

### 阶段② 重构应用骨架 ✅

**目标**：从"方盒子堆叠"改为"悬浮层叠"的现代桌面布局。

- 侧栏改为悬浮玻璃卡 + FAB 按钮唤起
- 应用标题栏改造为右上角状态胶囊（fixed 定位、blur(20px)、胶囊圆角）
- 新增顶部 44px 主拖拽带，各页面完成头部避让（聊天区 52px / 设置页 68px / 知识库 66px）
- 涉及：[App.vue](../frontend/src/App.vue)、[AppHeader.vue](../frontend/src/components/layout/AppHeader.vue)、[style.css](../frontend/src/style.css) 等

### 阶段③ 重构聊天核心体验 ✅

**目标**：聊天主界面对标 Claude / ChatGPT 的现代形态。

- **720px 居中消息流**：长屏下不再拉满全宽；限宽加在消息项自身，普通模式与虚拟滚动模式双兼容
- **AI 回复去气泡化**：正文直接躺在画布上（透明背景），身份识别交给头像；用户气泡保留芽绿
- **悬浮输入舱**：整条底部色带退役，输入框升级为玻璃舱体，聚焦时亮起主色光环
- 流式渲染链路（useMorphRender 分块渲染）零触碰，业务逻辑无回归
- 涉及：[MessageItem.vue](../frontend/src/components/chat/MessageItem.vue)、[MessageList.vue](../frontend/src/components/chat/MessageList.vue)、[MessageBubble.vue](../frontend/src/components/chat/MessageBubble.vue)、[ChatInput.vue](../frontend/src/components/chat/ChatInput.vue)

### 阶段④ 全局皮肤统一 ✅

**目标**：让所有"浮在应用上面"的元素统一穿上深空苗圃外衣。

- 下拉菜单、模态对话框补齐毛玻璃材质（blur 20px），纳入玻璃材质家族
- 启动屏氛围光晕、标题辉光及两组动画共 6 处由旧版 GitHub 蓝切换为芽绿
- 清理消息条 info 类型的死蓝兜底 ×2（变量已同步芽绿，兜底永不生效且具误导性）
- 侦察确认 discrete API 主题接线正确，避免一次不必要的重写
- 涉及：[style.css](../frontend/src/style.css)、[SplashScreen.vue](../frontend/src/components/layout/SplashScreen.vue)

### 阶段⑤ 次级页面巡检适配 ✅

**目标**：设置页（14 个子组件）与知识库页（3 个子组件）逐一对齐 v4 规范。

- **修复真 bug**：外观设置背景预览气泡引用了不存在的 `--accent-primary-rgb` 变量，一直在渲染旧版蓝色兜底值；改用 `color-mix` 直接取主色 70% 透明度，自动跟随亮暗主题
- **死代码清理**：模型下载器 14 处 `var(--error-color, …)` 死兜底删除（且原兜底值存在两个互相矛盾的版本）
- 巡检确认干净的部分：两个页面编排壳、知识库三件套（零硬编码）、各处圆角（均落在 v4 阶梯）、预览卡专用白纸模拟色（有意硬编码，正确保留）
- 涉及：[AppearanceSettings.vue](../frontend/src/components/settings/AppearanceSettings.vue)、[ModelDownloader.vue](../frontend/src/components/settings/ModelDownloader.vue)

### 阶段⑥ 全面验证 ✅

| 验证项 | 结果 |
|-------|------|
| 单元测试（Vitest） | 19 个测试文件，226 passed + 1 skipped，零回归 |
| 类型检查（vue-tsc） | 零错误 |
| 生产构建（vite build） | 4559 模块打包成功，产物含完整字体/CSS/JS 分包 |
| 代码规范（ESLint --max-warnings 0） | 零警告 |

---

## 四、质量保障机制

1. **每阶段验证制**：六个阶段各自完成后立即运行 typecheck + 全量测试，全程保持 226 测试基线零回归
2. **契约测试守护**：themeTokens（47 项）锁定调色板一致性；backgroundTheme（22 项源码断言）守护背景图模式约束
3. **业务逻辑零触碰原则**：所有改动限定于模板结构与样式层；虚拟滚动滞回启停（50 进 40 出）、流式分块渲染、discrete 弹窗链路均经验证未受影响
4. **残留扫描收口**：阶段⑤批量替换后以 grep 扫描发现 1 处漏网并补刀清零——工具会撒谎，扫描不会

## 五、修复与清理统计

| 类型 | 数量 | 明细 |
|------|-----|------|
| 真 bug 修复 | 1 处 | 背景预览气泡旧版蓝色（变量不存在导致兜底长期生效） |
| 死代码清理 | 16 处 | ModelDownloader 兜底 ×14 + 消息条死蓝 ×2 |
| 不必要重写的规避 | 1 次 | discrete.ts 侦察确认接线正确，未做任何修改 |

## 六、后续建议

1. **人工验收**：运行 `wails dev` 重点体验——启动屏辉光、居中消息流、输入舱聚焦光环、菜单/弹窗毛玻璃、背景图模式下的材质通透度
2. **可选打磨方向**：
   - 亮色主题下的玻璃材质对比度实测（当前优化重心在暗色）
   - 状态胶囊在窄窗口下的自适应表现
3. **维护约定**：新增颜色一律进 tokens.css 令牌体系并由契约测试覆盖；新浮起组件直接套用「玻璃材质公式」四件套

---

*本报告由六阶段重构过程的会话档案整理而成。*
