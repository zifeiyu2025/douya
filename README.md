<div align="center">

  <img src="build/appicon.png" alt="豆芽 Douya" width="120" height="120" />

# 豆芽 Douya

**隐私优先的本地 AI 桌面助手 · 基于 llama.cpp · 完全离线**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-v0.11.8-blue.svg)](https://github.com/zifeiyu2025/douya/releases)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js&logoColor=white)](https://vuejs.org/)
[![Wails](https://img.shields.io/badge/Wails-v2-red.svg)](https://wails.io/)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-0078D4?logo=windows&logoColor=white)](https://github.com/zifeiyu2025/douya/releases)
[![CUDA](https://img.shields.io/badge/GPU-CUDA-76B900?logo=nvidia&logoColor=white)](https://developer.nvidia.com/cuda-toolkit)

[特性](#-核心特性) · [快速开始](#-快速开始) · [下载](https://github.com/zifeiyu2025/douya/releases) · [文档](docs/external-tools-access.md) · [架构](#-技术架构)

</div>

---

> **豆芽**是一款基于 [llama.cpp](https://github.com/ggml-org/llama.cpp) 的本地大语言模型桌面客户端。所有推理在本地完成，无需联网、数据不出本机，真正守护你的隐私。同时原生提供 **OpenAI 兼容 API**，可作为本地推理后端接入 Claude Code、Codex、OpenCoder 等外部编程工具。

<div align="center">

<img src="tests/screenshots/home.png" alt="豆芽主界面" width="90%" />

<sub>图：豆芽主界面 · 流式对话 · 思考过程折叠 · 多模态附件</sub>

</div>

---

## ✨ 核心特性

### 🧠 本地推理引擎
- **多后端支持**：CUDA（NVIDIA）/ HIP（AMD）/ SYCL（Intel）/ Vulkan（跨厂商）/ OpenVINO / CPU，按 GPU 厂商自动推荐
- **后端在线下载**：从 llama.cpp GitHub Releases 自动下载对应后端运行时，按需解压，无需手动配置
- **GPU 微架构检测**：识别 Blackwell / Ada / Ampere / Turing，针对 RTX 50 系等新架构调优量化策略
- **智能硬件检测**：自动识别 GPU 型号与 VRAM，计算最优 GPU 层数、线程数、KV Cache 策略
- **GGUF 元数据解析**：按模型规模分级调优（block_count、embedding_length、chat_template）
- **高级优化**：Flash Attention、KV Cache 量化（Q4_0 / Q8_0）、mlock、DRY 采样器
- **投机解码**（Speculative Decoding）：草稿模型加速推理
- **mmproj GPU 卸载**：多模态投影模型自动走 GPU

### 🔄 智能模型管理
- **Router LRU 切换**：切换模型无需手动卸载，自动管理 VRAM
- **Sleep-Idle 休眠**：空闲自动释放 VRAM，请求到达自动唤醒
- **模型热重载**：`GET /models?reload` 无需重启服务
- **状态可视化**：`loaded` / `loading` / `sleeping` / `unloaded` 实时显示
- **能力预检测**：扫描 GGUF `clip.has_vision_encoder` / `clip.has_audio_encoder`，加载前即知模型能力
- **思考模式识别**：三级优先级（`/props supports_preserve_reasoning` > GGUF Architecture > 文件名关键词）

### 💬 智能对话
- **流式输出（SSE）**：逐字打字机式体验
- **多轮对话管理**：创建、重命名、删除、搜索、重生成
- **消息级联删除**：自动清理关联回复
- **对话导出**：支持文件导出
- **推理模型支持**：DeepSeek-R1 / QwQ 思考链可折叠
- **系统提示词**：支持日期变量注入
- **上下文溢出三层防御**：预防性裁剪 + 全量估算 + 错误后重试

### 🖼️ 多模态输入
- **图片**：Gemma、LLaVA 等视觉模型，支持粘贴 / 上传
- **音频**：音频多模态模型（如 Gemma 4 audio）
- **PDF**：`ledongthuc/pdf` 提取文本，正则回退兜底
- **文本**：直接读取 `.txt` / `.md`
- **智能附件过滤**：按 mmproj 实际能力自动启用 / 禁用
- **Token 精确估算**：图片按 3500 tokens 计算避免溢出

### 🔍 联网搜索
- **三态开关**：`off` / `auto`（智能）/ `on`（强制）
  - `auto`：强模型自主 Tool Call；弱模型预搜索 + 注入
- **步降式链路**：Tavily → Ollama → 360 → Bing（顺序尝试，不并发）
- **中文优化**：关键词加双引号精确匹配；DuckDuckGo `kl=cn-zh`、Bing `cc=cn&setlang=zh-Hans`
- **结果自然融入**：XML 标签注入，系统提示词约束不使用 `[1][2]` 编号
- **RAG 互斥**：RAG 开启时自动关闭联网搜索

### 📚 RAG 知识库
- **BadgerDB + HNSW** 向量存储，完全离线
- **混合检索**：向量相似度 + BM25 关键词
- **独立 Embedding 模型**：可配置专用模型，否则跟随聊天模型
- **维度自动适配**：向量集合维度 0 时自动更新为实际维度
- **多知识库**：可创建多个，自由切换启用
- **上下文隔离**：检索结果作为独立 system 消息注入，不污染原 prompt
- **并发安全**：独立 `ragMu` 读写锁保护

### 🔌 外部工具接入
- **OpenAI 兼容 API**：原生暴露 `/v1/chat/completions`、`/v1/embeddings` 等端点
- **暴露开关**：`0.0.0.0`（局域网）/ `127.0.0.1`（仅本机）一键切换
- **API Key 鉴权**：`Authorization: Bearer <key>`
- **已验证接入**：Claude Code、Codex、OpenCoder / Open Claw、通用 OpenAI 兼容客户端
- 详见 [docs/external-tools-access.md](docs/external-tools-access.md)

### 🎤 语音 · 🎨 个性化 · 🪟 桌面体验
- **语音输入**：Web Speech API，实时中间结果
- **深浅主题**、**自定义聊天背景**、**自定义头像**
- **生成参数**：Temperature / Top-P / Top-K / Min-P / Repeat Penalty
- **推理控制**：Reasoning（auto/on/off）、Budget、Format
- **无边框窗口**、**系统托盘**、**单实例保护**、**优雅退出**
- **链接外开**：所有 http(s) 链接走系统默认浏览器

### 🔒 安全加固
- **API Key AES-GCM 加密**：`enc:` 前缀标识
- **文件路径校验**：`filepath.Clean` + `..` 检测 + 扩展名白名单
- **上传校验**：扩展名白名单 + MIME 类型 + 200MB 大小限制
- **API Key 防暴露**：前端仅返回设置状态
- **XSS 防护**：`lightSanitize` 轻量消毒
- **搜索结果 XML 转义**：防注入攻击

---

## 🚀 快速开始

### 方式一：下载发布包（推荐）

前往 [Releases](https://github.com/zifeiyu2025/douya/releases) 下载 `Douya-vX.X.X-win64.zip`，解压即可使用，**无需安装**。

**首次使用：**
1. 解压 zip 到任意目录
2. 将 GGUF 模型文件放入 `models/` 目录
3. 双击运行 `bin/Douya.exe`
4. 程序会在 `data/` 目录自动创建数据库和配置

### 方式二：从源码构建

**环境要求**
- Go 1.25+、Node.js 18+、Windows 10/11
- NVIDIA GPU（推荐，VRAM ≥ 6GB）+ CUDA 12+
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`

```bash
# 开发模式
cd frontend && npm install && cd ..
wails dev
```

```powershell
# 生产构建（生成 release/ 发布包）
.\build.ps1
```

> `build.ps1` 优先使用 `D:\Program Files\GoTools\bin\wails.exe`，回退到 `$GOPATH/bin/wails.exe`。脚本包含 UTF-8 BOM 以兼容 PowerShell 5.1。

### 接入外部编程工具

1. 「设置 → 服务 API KEY」开启「暴露服务器地址」
2. 设置 API Key（防止未授权访问）
3. 重启服务使配置生效
4. 参阅 [docs/external-tools-access.md](docs/external-tools-access.md) 配置工具

**接入三要素：**
- **Base URL**：`http://<豆芽机器IP>:8080/v1`
- **API Key**：豆芽设置中配置的 Key
- **Model**：默认模型或指定模型名

---

## 🛠️ 技术架构

### 技术栈

| 层级 | 技术 |
|------|------|
| 桌面框架 | Go 1.25 + [Wails v2](https://wails.io/) |
| 前端 | Vue 3 + TypeScript + [Naive UI](https://www.naiveui.com/) + Pinia v4 |
| 推理引擎 | [llama.cpp](https://github.com/ggml-org/llama.cpp)（多后端：CUDA/HIP/SYCL/Vulkan/OpenVINO/CPU，Router 模式） |
| 向量存储 | [BadgerDB v4](https://github.com/dgraph-io/badger)（HNSW 索引） |
| 数据库 | SQLite3 |
| PDF 解析 | [ledongthuc/pdf](https://github.com/ledongthuc/pdf) |
| 日志 | [zerolog](https://github.com/rs/zerolog) |
| 系统托盘 | [systray](https://github.com/fyne-io/systray) |
| 构建工具 | Vite + vue-tsc |

### 项目结构

```
douya/
├── main.go                     # 应用入口：Wails 初始化、单实例、系统托盘
├── app.go                      # 核心逻辑：模型切换、服务管理、Wails 绑定
├── app_*.go                    # 应用层模块（chat/config/lifecycle/rag/search/server）
├── build.ps1                   # Windows 构建脚本（生成 release/ 发布包）
├── internal/
│   ├── chat/                   # 对话服务：消息构建、流式生成、搜索集成、RAG
│   ├── config/                 # 配置管理：JSON 持久化、参数验证
│   ├── llm/                    # LLM 控制层：client / server / backend / preset / vram / ringbuffer
│   ├── rag/                    # RAG 知识库：vector_store / bm25 / document_pipeline
│   ├── search/                 # 搜索引擎：Tavily / Ollama / Bing（步降式）
│   ├── secrets/                # AES-GCM 加密
│   ├── store/                  # SQLite 存储：对话、消息、设置
│   ├── system/                 # 硬件检测、GGUF 元数据、智能参数
│   ├── pdfutil/                # PDF 文本提取（带正则回退）
│   ├── httputil/               # HTTP 工具
│   ├── pathutil/               # 路径工具
│   ├── apperror/               # 错误处理
│   ├── logger/                 # 日志配置
│   └── version/                # 版本信息
├── frontend/
│   └── src/
│       ├── App.vue             # 根组件
│       ├── components/         # 组件（ChatView/MessageItem/Sidebar/...）
│       ├── views/              # 视图（Settings/Knowledge）
│       ├── stores/             # Pinia 状态管理
│       ├── composables/        # 组合式函数
│       ├── services/          # Wails 绑定层
│       ├── utils/              # 工具（Markdown/消毒/模型引用）
│       └── types/              # 类型定义
├── tests/                      # 测试用例（含 -race 竞态检测）
│   └── screenshots/            # 程序截图
├── docs/                       # 文档
├── build/                      # Wails 构建配置 + 图标
└── scripts/                    # 辅助脚本（版本一致性检查、pre-commit）
```

### 架构亮点

**多后端管理**：运行时按后端类型分子目录（`runtime/cuda/`、`runtime/vulkan/` 等），切换后端时按需从 llama.cpp GitHub Releases 下载解压。`backend_type=auto` 时根据 GPU 厂商（NVIDIA/AMD/Intel）自动推荐最合适的原生后端，无 GPU 时回退 CPU。

**Router 模式模型切换**：利用 llama.cpp Router 的 LRU 自动卸载机制，切换模型无需手动 `UnloadModel` + 等待 VRAM 释放。Sleep-Idle 让空闲模型自动休眠，新请求自动唤醒。启动加 `--no-models-autoload` 避免与显式 `/models/load` 冲突。

**智能参数计算**：解析 GGUF 元数据 → 估算模型规模等级（Tiny/Small/Medium/Large/XL）→ 结合 GPU VRAM 自动计算 GPU 层数、KV Cache 量化、Flash Attention。VRAM/模型 ≤0.7 → q8_0/q4_0；0.7~1.5 → q8_0/turbo3；>1.5 → turbo3/turbo2。

**多模态能力检测**：三层递进式——GGUF 预扫描（`clip.has_vision_encoder`）→ `/v1/models` 端点（`input_modalities`）→ `/props` 端点（最终依据）。

**思考模式检测**：三级优先级——`/props supports_preserve_reasoning` > GGUF Architecture（qwen3/gemma2/gemma4/llama4/phi4/deepseek）> 文件名关键词。

**Tool Call 支持检测**：基于 `/props` 的 `chat_template_caps.supports_tools`，替代弱模型白名单。

**会话异常恢复**：启动时自动扫描数据库，清理「用户消息已发送但无 AI 回复」的不完整对话。

---

## ⚙️ 配置说明

配置文件 `config.json` 在 `data/` 目录自动生成，加载时自动调用 `Validate()` 校验，失败回退默认配置。

### 模型与推理

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `model_path` | 默认模型路径 | （空） |
| `backend_type` | 计算后端（auto / cuda / hip / sycl / vulkan / openvino / cpu） | `"auto"` |
| `context_size` | 上下文窗口大小 | `8192` |
| `port` | llama-server 端口 | `8080` |
| `expose_server` | 暴露服务器地址（局域网访问） | `false` |
| `mmproj_auto` | 自动检测 mmproj | `true` |
| `mmproj_offload` | mmproj GPU 卸载 | `true` |
| `sleep_idle_seconds` | 模型空闲休眠超时（-1 禁用） | `-1` |
| `models_max` | 最大并行模型数 | `1` |

### 生成参数

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `temperature` | 生成温度（越高越随机） | `0.8` |
| `top_p` | 核采样阈值 | `0.95` |
| `top_k` | Top-K 采样 | `40` |
| `min_p` | 最小概率阈值 | `0.05` |
| `repeat_penalty` | 重复惩罚系数 | `1` |
| `dry_multiplier` | DRY 采样器强度 | `0.0` |

### 推理控制

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `reasoning` | 推理模式（auto / on / off） | `"off"` |
| `reasoning_budget` | 推理 Token 预算 | `0` |
| `reasoning_format` | 推理输出格式 | （空） |
| `thinking_enabled` | 思考过程开关 | `true` |

### 高级优化

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `kv_unified` | 统一 KV Cache | `false` |
| `kv_offload` | KV Cache GPU 卸载 | `true` |
| `cache_type_k` / `cache_type_v` | KV Cache 量化类型 | `"f16"` |
| `spec_type` | 投机解码草稿模型类型 | （空） |
| `context_shift` | 上下文移位开关 | `false` |
| `mmap` | 内存映射加载 | `true` |

### 搜索与 RAG

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `search_mode` | 搜索模式（off / auto / on） | `"off"` |
| `rag_enabled` | RAG 知识库开关 | `false` |
| `rag_top_k` | 知识库召回数量 | `3` |
| `rag_min_score` | 召回最低相似度 | `0.3` |
| `rag_chunk_size` | 文档分块大小 | `512` |
| `rag_chunk_overlap` | 分块重叠大小 | `64` |

### 个性化

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `system_prompt` | 自定义系统提示词 | （空） |
| `system_prompt_mode` | 提示词模式（append / replace） | `"append"` |
| `chat_background` | 聊天背景图片路径 | （空） |
| `chat_background_opacity` | 背景图片透明度 | `0.9` |
| `user_avatar` / `ai_avatar` | 头像路径 | （空） |

---

## 🧪 测试

项目采用测试驱动开发（TDD），关键模块均有测试覆盖：

```bash
# Go 测试（含竞态检测）
go test ./... -race

# 前端测试
cd frontend && npm test
```

**覆盖范围：**
- **对话**：功能、交互、质量、架构、注入保护、数据竞态、停止保存
- **配置**：Validate 校验、DefaultConfig、Load 容错
- **LLM**：客户端、预设、能力检测、环形缓冲区、服务端
- **RAG**：向量存储、文档管道、BM25、端到端
- **搜索**：链式降级、Provider 接口
- **前端**：Markdown 渲染、流式渲染、滚动控制、状态管理

---

## 📦 发布包结构

```
Douya-vX.X.X/
├── bin/
│   └── Douya.exe               # 主程序（双击运行）
├── data/                       # 数据目录（自动创建）
│   ├── douya.db                # SQLite 数据库
│   ├── .enc_key               # API Key 加密密钥（请勿删除）
│   └── rag/                    # RAG 向量索引
├── docs/
│   └── external-tools-access.md
├── models/                     # 模型文件目录（需自行放入 GGUF）
└── runtime/                    # llama-server.exe + 后端 DLL（按后端分子目录）
    └── cuda/                   # CUDA 后端运行时（首次切换其他后端时自动下载）
```

---

## 📋 平台支持

- **当前平台**：仅支持 Windows 10 / 11
- **运行时稳定性**：已优先保证

---

## 🤝 参与贡献

欢迎通过 [Issues](https://github.com/zifeiyu2025/douya/issues) 反馈问题或建议功能。提交 PR 前请确保：

```bash
go test ./... -race        # Go 测试通过
cd frontend && npm test    # 前端测试通过
cd frontend && npm run build  # 前端构建成功
```

---

## 📄 许可证

[MIT](LICENSE) © 2026 zifeiyu2025

---

<div align="center">

**[⭐ Star](https://github.com/zifeiyu2025/douya)** · **[📦 下载](https://github.com/zifeiyu2025/douya/releases)** · **[📖 文档](docs/external-tools-access.md)** · **[🐛 反馈](https://github.com/zifeiyu2025/douya/issues)**

</div>
