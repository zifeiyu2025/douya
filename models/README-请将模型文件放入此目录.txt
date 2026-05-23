此目录存放 GGUF 格式的模型文件。

目录结构示例：
models/
├── Gemma-4-E4B-U-Q4_K_M/
│   ├── Gemma-4-E4B-U-Q4_K_M.gguf      — 主模型文件
│   └── mmproj-Gemma-4-E4B-U-Q4_K_M.gguf — 多模态投影文件（可选）
├── Qwen3-8B-Q4_K_M/
│   ├── Qwen3-8B-Q4_K_M.gguf
│   └── mmproj-Qwen3-8B-Q4_K_M.gguf
└── DeepSeek-R1-7B-Q4_K_M.gguf

支持的模型来源：
- https://huggingface.co/models?search=gguf
- https://huggingface.co/ggml-org

模型文件较大，不纳入 Git 版本控制，需手动下载放置。
