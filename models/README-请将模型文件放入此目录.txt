此目录存放 GGUF 格式的模型文件。

优先推荐使用豆芽内置的模型下载器（无需手动下载）：
打开应用 → 设置 → 模型下载 → 搜索并下载。
支持 ModelScope 魔搭社区 与 HF 国内镜像（hf-mirror.com），
下载自动断点续传，断网可重试。

如需手动放置模型文件，目录结构示例：
models/
├── Gemma-4-E4B-U-Q4_K_M/
│   ├── Gemma-4-E4B-U-Q4_K_M.gguf      — 主模型文件
│   └── mmproj-Gemma-4-E4B-U-Q4_K_M.gguf — 多模态投影文件（可选）
├── Qwen3-8B-Q4_K_M/
│   ├── Qwen3-8B-Q4_K_M.gguf
│   └── mmproj-Qwen3-8B-Q4_K_M.gguf
└── DeepSeek-R1-7B-Q4_K_M.gguf

支持的模型来源（国内访问建议使用镜像/国内站）：
- https://hf-mirror.com/models?search=gguf（HuggingFace 国内镜像，速度快）
- https://modelscope.cn（ModelScope 魔搭社区）
- https://huggingface.co/models?search=gguf（官方站，国内可能较慢）

模型文件较大，不纳入 Git 版本控制，需手动下载放置。
