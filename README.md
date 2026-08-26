<p align="right"><b><a href="README.zh-CN.md">中文</a></b> | English</p>

<div align="center">

  <img src="build/appicon.png" alt="Douya" width="120" height="120" />

# Douya

**A privacy-first local AI desktop assistant · Built on llama.cpp · Fully offline**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26.3-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js&logoColor=white)](https://vuejs.org/)
[![Wails](https://img.shields.io/badge/Wails-v2-red.svg)](https://wails.io/)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-0078D4?logo=windows&logoColor=white)](https://github.com/zifeiyu2025/douya/releases)
[![CUDA](https://img.shields.io/badge/GPU-CUDA-76B900?logo=nvidia&logoColor=white)](https://developer.nvidia.com/cuda-toolkit)

[Features](#-core-features) · [Quick Start](#-quick-start) · [Download](https://github.com/zifeiyu2025/douya/releases) · [Docs](docs/external-tools-access.md) · [Architecture](#-technical-architecture)

</div>

---

> **Douya** is a local large language model desktop client built on [llama.cpp](https://github.com/ggml-org/llama.cpp). All inference happens locally — no network connection required and no data ever leaves your machine, truly protecting your privacy. It also natively exposes an **OpenAI-compatible API**, so you can use it as a local inference backend for external coding tools such as Claude Code, Codex, OpenCoder, and more.

<div align="center">

<img src="tests/screenshots/home.png" alt="Douya main interface" width="90%" />

<sub>Douya main interface · Streaming chat · Collapsible reasoning · Multimodal attachments</sub>

</div>

---

## ✨ Core Features

### 🧠 Local Inference Engine
- **Multi-backend support**: CUDA (NVIDIA) / HIP (AMD) / SYCL (Intel) / Vulkan (cross-vendor) / OpenVINO / CPU, with automatic recommendation based on your GPU vendor
- **Backend online download**: automatically downloads the matching backend runtime from llama.cpp GitHub Releases and unpacks on demand — no manual setup
- **GPU micro-architecture detection**: detects Blackwell / Ada / Ampere / Turing and tunes quantization strategies for newer architectures like the RTX 50 series
- **Smart hardware detection**: automatically identifies the GPU model and VRAM, then computes optimal GPU layers, thread counts, and KV Cache strategy
- **GGUF metadata parsing**: tiered tuning by model size (`block_count`, `embedding_length`, `chat_template`)
- **Advanced optimizations**: Flash Attention, KV Cache quantization (Q4_0 / Q8_0), mlock, DRY sampler
- **Speculative decoding**: draft models to accelerate inference
- **mmproj GPU offload**: multimodal projection models automatically run on GPU

### 🔄 Smart Model Management
- **Router LRU switching**: switch models without manual unload — VRAM is managed automatically
- **Sleep-Idle**: releases VRAM when idle, auto-wakes on incoming requests
- **Hot model reload**: `GET /models?reload` — no server restart needed
- **State visualization**: real-time display of `loaded` / `loading` / `sleeping` / `unloaded`
- **Capability pre-detection**: scans GGUF `clip.has_vision_encoder` / `clip.has_audio_encoder` to know model capabilities before loading
- **Reasoning mode detection**: three-level priority (`/props supports_preserve_reasoning` > GGUF Architecture > filename keywords)

### 💬 Smart Chat
- **Streaming output (SSE)**: typewriter-style experience, token by token
- **Multi-turn conversation management**: create, rename, delete, search, regenerate
- **Message cascade deletion**: related replies are cleaned up automatically
- **Conversation export**: supported via file export
- **Reasoning model support**: DeepSeek-R1 / QwQ thinking chains are collapsible
- **KV cache persistence**: auto-saves / restores the KV Cache per conversation for smoother continued chats
- **System prompts**: supports date variable injection
- **Three-layer context overflow defense**: preventive trimming + full estimation + retry after error

### 🖼️ Multimodal Input
- **Images**: vision models such as Gemma, LLaVA — paste or upload
- **Audio**: audio multimodal models (e.g., Gemma 4 audio)
- **PDF**: text extraction via `ledongthuc/pdf`, with regex fallback
- **Text**: read `.txt` / `.md` directly
- **Smart attachment filtering**: auto-enabled/disabled based on the actual mmproj capabilities
- **Accurate token estimation**: images counted as 3500 tokens to avoid overflow

### 🔍 Web Search
- **Three-state switch**: `off` / `auto` (smart) / `on` (forced)
  - `auto`: strong models use Tool Calls on their own; weak models pre-search + inject
- **Step-down chain**: Tavily → Ollama (sequential attempts, no concurrency)
- **Chinese optimization**: keywords wrapped in double quotes for exact match; DuckDuckGo `kl=cn-zh`, Bing `cc=cn&setlang=zh-Hans`
- **Natural result integration**: XML tag injection; system prompt constrains the model from using `[1][2]` numbering
- **RAG mutual exclusion**: web search is disabled automatically when RAG is enabled

### 📚 RAG Knowledge Base
- **BadgerDB + HNSW** vector store, fully offline
- **Hybrid retrieval**: vector similarity + BM25 keyword search
- **Dedicated embedding model**: configurable; otherwise falls back to the chat model
- **Automatic dimension adaptation**: vector collections with dimension 0 are updated to the actual dimension
- **Multiple knowledge bases**: create several and freely toggle which is enabled
- **Context isolation**: retrieval results are injected as separate system messages without polluting the original prompt
- **Concurrency safety**: protected by an independent `ragMu` read-write lock

### 🔌 External Tool Access
- **OpenAI-compatible API**: natively exposes `/v1/chat/completions`, `/v1/embeddings`, and more endpoints
- **One-click API key generation**: `sk-douya-` prefix + random characters, encrypted with AES-GCM
- **Exposure switch**: `0.0.0.0` (LAN) / `127.0.0.1` (local only) — switch with one click
- **API key auth**: `Authorization: Bearer <key>`
- **Verified integrations**: Claude Code, Codex, OpenCoder / Open Claw, and generic OpenAI-compatible clients
- See [docs/external-tools-access.md](docs/external-tools-access.md) for details

### 🎤 Voice · 🎨 Personalization · 🪟 Desktop Experience
- **Voice input**: Web Speech API with real-time intermediate results
- **Light/dark themes**, **custom chat backgrounds**, **custom avatars**
- **Generation parameters**: Temperature / Top-P / Top-K / Min-P / Repeat Penalty
- **Reasoning controls**: Reasoning (auto/on/off), Budget, Format
- **Frameless window**, **system tray**, **single-instance protection**, **graceful exit**
- **Open links externally**: all http(s) links open in the system default browser

### 🔒 Security Hardening
- **API key AES-GCM encryption**: marked with the `enc:` prefix
- **File path validation**: `filepath.Clean` + `..` detection + extension whitelist
- **Upload validation**: extension whitelist + MIME type + 200MB size limit
- **API key exposure prevention**: the frontend only receives the status
- **XSS protection**: lightweight `lightSanitize` sanitization
- **Search result XML escaping**: prevents injection attacks

---

## 🚀 Quick Start

### Option 1: Download a release (recommended)

Download `Douya-vX.X.X-windows.zip` from [Releases](https://github.com/zifeiyu2025/douya/releases), unzip, and run — **no installation required**.

**First run:**
1. Unzip to any directory
2. Put your GGUF model files into the `models/` directory
3. Double-click `bin/Douya.exe`
4. The program creates the database and configuration automatically in `data/`

### Option 2: Build from source

**Requirements**
- Go 1.26+, Node.js 18+, Windows 10/11
- NVIDIA GPU (recommended, VRAM ≥ 6GB) + CUDA 12+
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

```bash
# Development mode
cd frontend && npm install && cd ..
wails dev
```

```powershell
# Production build (generates the release/ package)
.\build.ps1
```

> `build.ps1` prefers `D:\Program Files\GoTools\bin\wails.exe` and falls back to `$GOPATH/bin/wails.exe`. The script includes a UTF-8 BOM for PowerShell 5.1 compatibility.

### Connect external coding tools

1. In "Settings → Service API Key", enable "Expose server address"
2. Generate an API Key (to prevent unauthorized access)
3. Restart the service to apply the configuration
4. Configure the tool by following [docs/external-tools-access.md](docs/external-tools-access.md)

**The three essentials:**
- **Base URL**: `http://<Douya machine IP>:8080/v1`
- **API Key**: the key configured in Douya settings
- **Model**: the default model or a specific model name

---

## 🛠️ Technical Architecture

### Tech Stack

| Layer | Technology (pinned versions) |
|------|------|
| Desktop framework | Go 1.26.3 + [Wails v2](https://wails.io/) `v2.14.0` |
| Frontend | [Vue 3](https://vuejs.org/) `^3.5.41` + TypeScript + [Naive UI](https://www.naiveui.com/) `^2.44.1` + Pinia `^4.0.3` |
| Inference engine | [llama.cpp](https://github.com/ggml-org/llama.cpp) (multi-backend: CUDA/HIP/SYCL/Vulkan/OpenVINO/CPU, Router mode) |
| Vector store | [BadgerDB v4](https://github.com/dgraph-io/badger) `v4.9.6` (HNSW index) |
| Database | [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) `v1.14.49` |
| PDF parsing | [ledongthuc/pdf](https://github.com/ledongthuc/pdf) `v0.0.0-20250511` |
| Logging | [zerolog](https://github.com/rs/zerolog) `v1.35.1` |
| System tray | [fyne.io/systray](https://github.com/fyne-io/systray) `v1.12.2` |
| Build tooling | [Vite](https://vitejs.dev/) `^8.2.1` + vue-tsc `^3.3.10` |
| Other | WebSocket `v1.5.3`, UUID `v1.6.0`, conpty `v0.1.4` |

### Project Structure

```
douya/
├── main.go                     # App entry: Wails init, single instance, system tray
├── app.go                      # Core logic: model switching, service management, Wails bindings
├── app_*.go                    # App layer modules (chat/config/lifecycle/rag/search/server)
├── build.ps1                   # Windows build script (generates the release/ package)
├── internal/
│   ├── chat/                   # Chat service: message building, streaming, search integration, RAG
│   ├── config/                 # Configuration: JSON persistence, parameter validation
│   ├── llm/                    # LLM control layer: client / server / backend / preset / vram / ringbuffer
│   ├── rag/                    # RAG knowledge base: vector_store / bm25 / document_pipeline
│   ├── search/                 # Search engines: Tavily / Ollama (step-down)
│   ├── secrets/                # AES-GCM encryption
│   ├── store/                  # SQLite storage: conversations, messages, settings
│   ├── system/                 # Hardware detection, GGUF metadata, smart parameters
│   ├── pdfutil/                # PDF text extraction (with regex fallback)
│   ├── httputil/               # HTTP utilities
│   ├── pathutil/               # Path utilities
│   ├── distinfo/               # Distribution channel detection (portable / Microsoft Store) & old-install data migration
│   ├── apperror/               # Error handling
│   ├── logger/                 # Logging configuration
│   └── version/                # Version info
├── frontend/
│   └── src/
│       ├── App.vue             # Root component
│       ├── components/         # Components (ChatView/MessageItem/Sidebar/...)
│       ├── views/              # Views (Settings/Knowledge)
│       ├── stores/             # Pinia state management
│       ├── composables/        # Composable functions
│       ├── services/          # Wails binding layer
│       ├── utils/              # Utilities (Markdown/sanitizer/model refs)
│       └── types/              # Type definitions
├── tests/                      # Test suite (incl. -race race detection)
│   └── screenshots/            # App screenshots
├── docs/                       # Documentation
├── build/                      # Wails build config + icons (incl. windows/msix store manifests)
└── scripts/                    # Helper scripts (version consistency check, pre-commit)
```

### Distribution Channels: Portable vs Microsoft Store

Douya ships through two distribution channels, identified uniformly by `internal/distinfo` (based on whether the exe path resides under `\WindowsApps\`):

| Capability | Release (portable) | Microsoft Store |
|------|--------------|-----------|
| Full features | ✅ Complete | ✅ Provided |
| Data directory | exe-adjacent `data/` | `%LOCALAPPDATA%\Douya` (the WindowsApps install directory is read-only) |
| In-app self-update | ✅ Supported | ❌ Soft-blocked (Store policy 10.1.5; updates go through the Store) |
| Legacy data migration | N/A | On first launch, `config.json` and `data/` from the previous install directory are migrated automatically |

> Before releasing, run `scripts\check_version_consistency.ps1` to verify version consistency: the main version (version.go, package.json) must be three-segment and strictly equal across sources; the exe file properties (wails.json ProductVersion) and the MSIX manifest (AppxManifest.xml Identity Version) are four-segment x.y.z.n whose first three segments must equal the main version.

### Architecture Highlights

**Multi-backend management**: backends are stored in per-type subdirectories at runtime (`runtime/cuda/`, `runtime/vulkan/`, etc.). When switching backends, the matching runtime is downloaded from llama.cpp GitHub Releases and unpacked on demand. With `backend_type=auto`, the most suitable native backend is recommended automatically based on the GPU vendor (NVIDIA/AMD/Intel); it falls back to CPU when no GPU is present. If a backend switch fails, it automatically rolls back to the last successful configuration.

**Router mode model switching**: leverages llama.cpp Router's LRU auto-unload mechanism, so switching models doesn't require a manual `UnloadModel` + waiting for VRAM release. Sleep-Idle puts idle models to sleep automatically and wakes them on new requests. Start with `--no-models-autoload` to avoid conflicts with explicit `/models/load`.

**Smart parameter calculation**: parses GGUF metadata → estimates the model size tier (Tiny/Small/Medium/Large/XL) → combined with GPU VRAM, automatically computes GPU layers, KV Cache quantization, and Flash Attention. VRAM/model ≤0.7 → q8_0/q4_0; 0.7~1.5 → q8_0/turbo3; >1.5 → turbo3/turbo2.

**Multimodal capability detection**: three-tier progressive — GGUF pre-scan (`clip.has_vision_encoder`) → `/v1/models` endpoint (`input_modalities`) → `/props` endpoint (final authority).

**Reasoning mode detection**: three-level priority — `/props supports_preserve_reasoning` > GGUF Architecture (qwen3/gemma2/gemma4/llama4/phi4/deepseek) > filename keywords.

**Reasoning effort control**: passed natively to the template via the request-level `ReasoningEffort` field from llama-server, without manually injecting prompts; hard-thinking models (e.g., the DeepSeek family) always reserve a generous token budget.

**Tool call capability detection**: based on `/props` `chat_template_caps.supports_tools`, replacing weak-model whitelists.

**Session exception recovery**: on startup, the database is scanned automatically to clean up incomplete conversations where the user message was sent but no AI reply was received.

---

## ⚙️ Configuration

The `config.json` file is auto-generated in `data/`, validated via `Validate()` on load, and falls back to defaults on failure.

### Model & Inference

| Key | Description | Default |
|--------|------|--------|
| `model_path` | Default model path | (empty) |
| `backend_type` | Compute backend (auto / cuda / hip / sycl / vulkan / openvino / cpu) | `"auto"` |
| `context_size` | Context window size | `8192` |
| `port` | llama-server port | `8080` |
| `expose_server` | Expose the server address (LAN access) | `false` |
| `mmproj_auto` | Auto-detect mmproj | `true` |
| `mmproj_offload` | Offload mmproj to GPU | `true` |
| `sleep_idle_seconds` | Model idle sleep timeout (-1 disables) | `-1` |
| `models_max` | Maximum parallel models | `1` |

### Generation Parameters

| Key | Description | Default |
|--------|------|--------|
| `temperature` | Sampling temperature (higher = more random) | `0.8` |
| `top_p` | Nucleus sampling threshold | `0.95` |
| `top_k` | Top-K sampling | `40` |
| `min_p` | Minimum probability threshold | `0.05` |
| `repeat_penalty` | Repetition penalty coefficient | `1` |
| `dry_multiplier` | DRY sampler strength | `0.0` |

### Reasoning Controls

| Key | Description | Default |
|--------|------|--------|
| `reasoning` | Reasoning mode (auto / on / off) | `"off"` |
| `reasoning_budget` | Reasoning token budget | `0` |
| `reasoning_format` | Reasoning output format | (empty) |
| `thinking_enabled` | Thinking-process toggle | `true` |

### Advanced Optimizations

| Key | Description | Default |
|--------|------|--------|
| `kv_unified` | Unified KV Cache | `false` |
| `kv_offload` | Offload KV Cache to GPU | `true` |
| `cache_type_k` / `cache_type_v` | KV Cache quantization type | `"f16"` |
| `spec_type` | Speculative decoding draft model type | (empty) |
| `context_shift` | Context shift toggle | `false` |
| `mmap` | Memory-mapped loading | `true` |

### Search & RAG

| Key | Description | Default |
|--------|------|--------|
| `search_mode` | Search mode (off / auto / on) | `"off"` |
| `rag_enabled` | RAG knowledge base toggle | `false` |
| `rag_top_k` | Knowledge base recall count | `3` |
| `rag_min_score` | Minimum recall similarity | `0.3` |
| `rag_chunk_size` | Document chunk size | `512` |
| `rag_chunk_overlap` | Chunk overlap size | `64` |

### Personalization

| Key | Description | Default |
|--------|------|--------|
| `system_prompt` | Custom system prompt | (empty) |
| `system_prompt_mode` | Prompt mode (append / replace) | `"append"` |
| `chat_background` | Chat background image path | (empty) |
| `chat_background_opacity` | Background image opacity | `0.9` |
| `user_avatar` / `ai_avatar` | Avatar paths | (empty) |

---

## 🧪 Testing

The project follows test-driven development (TDD), and key modules have test coverage:

```bash
# Go tests (with race detection)
go test ./... -race

# Frontend tests
cd frontend && npm test
```

**Coverage areas:**
- **Chat**: functionality, interaction, quality, architecture, injection protection, data races, stop & save
- **Config**: Validate, DefaultConfig, Load fault tolerance
- **LLM**: client, presets, capability detection, ring buffer, server
- **RAG**: vector store, document pipeline, BM25, end-to-end
- **Search**: chain fallback, provider interface
- **Frontend**: Markdown rendering, streaming render, scroll control, state management

---

## 📦 Release Package Structure

```
Douya-vX.X.X/
├── bin/
│   └── Douya.exe               # Main executable (double-click to run)
├── data/                       # Data directory (created automatically)
│   ├── douya.db                # SQLite database
│   ├── .enc_key                # API key encryption key (do not delete)
│   └── rag/                    # RAG vector index
├── docs/
│   └── external-tools-access.md
├── models/                     # Model directory (place GGUF files here)
└── runtime/                    # llama-server.exe + backend DLLs (per-backend subdirectories)
    └── cuda/                   # CUDA backend runtime (other backends downloaded on first use)
```

---

## 📋 Platform Support

- **Current platform**: Windows 10 / 11 only
- **Runtime stability**: prioritized

---

## 🤝 Contributing

Feedback and feature suggestions are welcome via [Issues](https://github.com/zifeiyu2025/douya/issues). Before submitting a PR, please make sure:

```bash
go test ./... -race        # Go tests pass
cd frontend && npm test    # Frontend tests pass
cd frontend && npm run build  # Frontend builds successfully
```

---

## 📄 License

[MIT](LICENSE) © 2026 zifeiyu2025

---

<div align="center">

**[⭐ Star](https://github.com/zifeiyu2025/douya)** · **[📦 Download](https://github.com/zifeiyu2025/douya/releases)** · **[📖 Docs](docs/external-tools-access.md)** · **[🐛 Feedback](https://github.com/zifeiyu2025/douya/issues)**

</div>
