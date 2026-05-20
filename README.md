# Douya - 璞嗚娊 本地AI

> 本地大语言模型桌面客户端，类似 Ollama，基于 Go + Wails + Vue3 构建。

## ✨ 功能特性

- 🚀 **本地推理** - 基于 llama.cpp，支持 GPU 加速 (CUDA)
- 🔄 **多模型切换** - 支持同时管理多个量化模型，一键切换
- 💬 **智能对话** - 流式输出、多轮对话、对话管理
- 🔍 **联网搜索** - 集成 Tavily / DuckDuckGo / Bing / GitHub 搜索
- 🖼️ **多模态** - 支持视觉模型 (Gemma 等)
- 🧠 **推理模式** - 支持 DeepSeek-R1 / QwQ 等推理模型
- ⚡ **智能调度** - 自动检测硬件、GPU 层数、线程优化
- 💾 **本地存储** - SQLite 存储对话历史

## 🛠️ 技术栈

| 层 | 技术 |
|---|------|
| 后端 | Go 1.25 + Wails v2 |
| 前端 | Vue 3 + TypeScript + Naive UI + Pinia |
| 推理 | llama.cpp (CUDA) |
| 存储 | SQLite3 |
| 构建 | Vite + vue-tsc |

## 📁 项目结构

`
douya/
├── main.go              # 入口
├── app.go               # 应用逻辑、模型路由
├── internal/
│   ├── chat/            # 对话服务
│   ├── config/          # 配置管理
│   ├── llm/             # llama-server 控制
│   ├── search/          # 搜索引擎集成
│   ├── store/           # SQLite 存储
│   ├── system/          # 硬件检测
│   └── tts/             # 语音合成
├── frontend/
│   └── src/             # Vue3 前端
├── engines/             # llama.cpp 引擎 (gitignore)
├── models/              # 模型文件 (gitignore)
└── runtime/             # CUDA 运行时 (gitignore)
`

## 🚀 快速开始

### 环境要求

- Go 1.25+
- Node.js 18+
- CUDA 12+ (GPU 加速)
- Windows 10/11

### 构建

`ash
# 安装前端依赖
cd frontend && npm install && cd ..

# 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 构建
wails build
`

### 运行

将 engines/、
untime/、models/ 目录放入构建输出目录，运行 douya.exe。

## 📄 License

MIT