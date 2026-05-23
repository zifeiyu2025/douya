# 豆芽 Douya — 本地 AI 桌面助手

<p align="center">
  <strong>隐私优先 · 完全离线 · GPU 加速 · 多模态</strong>
</p>

豆芽是一款基于 [llama.cpp](https://github.com/ggml-org/llama.cpp) 的本地大语言模型桌面客户端，所有推理均在本地完成，无需联网、无需 API Key、数据不出本机。

---

## ✨ 核心特性

### 🧠 本地推理引擎
- 基于 llama.cpp server（Router 模式），支持 CUDA GPU 加速
- 智能硬件检测：自动识别 GPU 型号与 VRAM，计算最优 GPU 层数、线程数、KV Cache 策略
- GGUF 元数据解析：自动读取模型架构信息，按模型规模分级调优参数
- Flash Attention、KV Cache 量化（Q4_0/Q8_0）、mlock 等高级优化一键开启

### 🔄 智能模型管理
- **多模型切换**：基于 llama.cpp Router 的 LRU 自动卸载机制，切换模型时无需手动卸载旧模型
- **Sleep-Idle 休眠**：模型空闲后自动释放 VRAM，新请求到来时自动唤醒
- **模型热重载**：支持 `GET /models?reload` 热更新模型列表，无需重启服务
- **模型状态可视化**：实时显示 loaded / loading / sleeping / unloaded 状态
- **mmproj 能力预检测**：扫描 GGUF 元数据中的 `clip.has_vision_encoder` / `clip.has_audio_encoder`，模型加载前即可知道多模态能力

### 💬 智能对话
- 流式输出（SSE），实时显示生成内容
- 多轮对话管理，支持对话重命名、删除、搜索
- 消息删除与重新生成（级联删除关联的 assistant 回复）
- 推理模型支持：DeepSeek-R1 / QwQ 等思考链模型，折叠显示推理过程
- 系统提示词自定义，支持日期变量注入

### 🖼️ 多模态输入
- **图片输入**：支持 Gemma、LLaVA 等视觉模型，粘贴或上传图片
- **音频输入**：支持音频多模态模型（如 Gemma 4 audio）
- **PDF 文档**：提取 PDF 文本内容作为上下文
- **文本文件**：直接读取 .txt / .md 等文本文件
- 智能附件过滤：根据模型实际能力（mmproj 是否加载）自动启用/禁用对应附件类型

### 🔍 联网搜索
- 多搜索引擎链式聚合：Tavily → DuckDuckGo → Bing → GitHub
- 按类别路由：通用搜索与代码搜索使用不同引擎
- 搜索结果注入对话上下文，LLM 基于搜索结果回答
- 一键开关联网搜索

### 📚 RAG 知识库
- 基于 BadgerDB 的本地向量存储（HNSW 索引）
- 支持文档导入、分块、向量化
- 使用本地 LLM 生成 Embedding，完全离线

### 🎤 语音输入
- 基于 Web Speech API 的语音识别
- 实时中间结果展示，支持随时停止

### 🎨 个性化
- 深色/浅色主题切换
- 自定义聊天背景、用户头像、AI 头像
- 生成参数调节：Temperature、Top-P、Top-K、Repeat Penalty
- 推理参数控制：Reasoning 模式（auto/on/off）、Reasoning Budget、Reasoning Format

---

## 🛠️ 技术栈

| 层级 | 技术 |
|------|------|
| 桌面框架 | Go 1.25 + [Wails v2](https://wails.io/) |
| 前端 | Vue 3 + TypeScript + [Naive UI](https://www.naiveui.com/) + Pinia |
| 推理引擎 | [llama.cpp](https://github.com/ggml-org/llama.cpp) (CUDA, Router 模式) |
| 向量存储 | [BadgerDB v4](https://github.com/dgraph-io/badger) (HNSW) |
| 数据库 | SQLite3 |
| 日志 | [zerolog](https://github.com/rs/zerolog) |
| 构建 | Vite + vue-tsc |

---

## 📁 项目结构

```
douya/
├── main.go                  # 应用入口
├── app.go                   # 核心逻辑：模型切换、服务管理、Wails 绑定
├── internal/
│   ├── chat/                # 对话服务：消息构建、流式生成、搜索集成、RAG
│   ├── config/              # 配置管理：JSON 持久化、参数验证
│   ├── llm/                 # LLM 客户端与服务端控制
│   │   ├── client.go        # HTTP 客户端：流式聊天、模型加载/卸载/重载
│   │   ├── server.go        # llama-server 进程管理、健康检查、自动重启
│   │   ├── preset.go        # 模型扫描、GGUF mmproj 能力检测、INI 生成
│   │   ├── types.go         # 数据类型：ModelCapabilities、ServerStatus
│   │   └── vram.go          # VRAM 释放检测（nvidia-smi）
│   ├── search/              # 搜索引擎：Tavily / DuckDuckGo / Bing / GitHub / Ollama
│   ├── rag/                 # RAG：BadgerDB 向量存储、文档管道、Embedder 适配
│   ├── store/               # SQLite 存储：对话、消息 CRUD
│   └── system/              # 硬件检测、GGUF 元数据解析、智能参数计算
├── frontend/
│   └── src/
│       ├── App.vue          # 根组件：模型选择器、服务状态
│       ├── components/      # ChatInput、MessageList、Sidebar、ThinkBlock 等
│       ├── views/           # SettingsView 设置页
│       ├── stores/          # Pinia 状态管理：chat、settings、theme
│       ├── services/        # Wails 绑定层
│       └── utils/           # Markdown 渲染、UTF-8 修复、模型名格式化
├── engines/                 # llama-server 可执行文件 (gitignore)
├── models/                  # GGUF 模型文件 (gitignore)
└── runtime/                 # CUDA 运行时库 (gitignore)
```

---

## 🚀 快速开始

### 环境要求

- **Go** 1.25+
- **Node.js** 18+
- **CUDA** 12+（GPU 加速，可选）
- **Windows** 10/11
- **NVIDIA GPU**（推荐，VRAM ≥ 6GB）

### 开发模式

```bash
# 1. 安装前端依赖
cd frontend && npm install && cd ..

# 2. 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 3. 开发模式运行
wails dev
```

### 生产构建

```bash
wails build
```

构建产物在 `build/bin/` 目录。将 `engines/`、`runtime/`、`models/` 目录放入同一目录后运行 `douya.exe`。

### 目录结构要求

```
douya.exe
├── config.json              # 自动生成
├── engines/
│   └── llama-server.exe     # llama.cpp 编译产物
├── models/                  # GGUF 模型文件
│   ├── model-a/
│   │   ├── model-a.Q4_K_M.gguf
│   │   └── mmproj-model-a.gguf   # 多模态投影文件（可选）
│   └── model-b.gguf
└── runtime/                 # CUDA 运行时
    ├── cudart64_12.dll
    └── ...
```

---

## ⚙️ 配置说明

配置文件 `config.json` 在应用根目录自动生成，主要配置项：

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `model_path` | 默认模型路径 | `models/Gemma-4-E4B-U-Q4_K_M/...` |
| `mmproj_auto` | 自动检测 mmproj 文件 | `true` |
| `mmproj_offload` | mmproj GPU 卸载 | `true` |
| `port` | llama-server 端口 | `8080` |
| `context_size` | 上下文窗口大小 | `32768` |
| `temperature` | 生成温度 | `0.8` |
| `reasoning` | 推理模式 | `"auto"` |
| `sleep_idle_seconds` | 模型休眠超时 | `120` |
| `models_max` | 最大并行模型数 | `1` |
| `search_enabled` | 联网搜索开关 | `false` |

---

## 🏗️ 架构亮点

### Router 模式模型切换

豆芽使用 llama.cpp 的 Router 模式管理模型生命周期：

```
用户切换模型 → LoadModel(newModel)
                    ↓
         router 内部 unload_lru()
         （自动卸载最久未用的模型）
                    ↓
         启动新子进程加载模型
                    ↓
         WaitForModelLoaded → 就绪
```

- 无需手动 `UnloadModel` + `WaitForVRAMRelease`
- LRU 策略自动管理 VRAM
- Sleep-Idle 机制：空闲模型自动休眠释放 VRAM，新请求自动唤醒

### 智能参数计算

根据 GPU VRAM 和模型规模自动计算最优参数：

1. 解析 GGUF 元数据获取模型架构（block_count、embedding_length）
2. 估算模型规模等级（Tiny / Small / Medium / Large / XL）
3. 结合 GPU VRAM 计算 GPU 层数、KV Cache 类型、Flash Attention 等

### 多模态能力检测

三层能力检测确保准确性：

1. **GGUF 预扫描**：读取 mmproj 文件的 `clip.has_vision_encoder` / `clip.has_audio_encoder`
2. **/v1/models 端点**：获取模型声明的 input_modalities
3. **/props 端点**：获取 mmproj 实际加载后的可用 modalities（最终依据）

---

## 📄 License

[MIT](LICENSE)
