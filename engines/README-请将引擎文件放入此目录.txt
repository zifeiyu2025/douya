此目录存放 llama.cpp 推理引擎的二进制文件。

需要将以下文件放入此目录：
- llama-server.exe    — llama.cpp server 主程序（Router 模式）
- llama.dll           — llama.cpp 核心库
- llama-common.dll    — 公共工具库
- llama-server-impl.dll — server 实现库
- ggml.dll            — GGML 基础库
- ggml-base.dll       — GGML 基础算子
- ggml-cpu.dll        — GGML CPU 后端
- ggml-cuda.dll       — GGML CUDA 后端
- mtmd.dll            — 多模态处理库

这些文件需要从 llama.cpp 项目编译获取：
https://github.com/ggml-org/llama.cpp

编译命令参考：
cmake -B build -DGGML_CUDA=ON -DLLAMA_SERVER=ON
cmake --build build --config Release
