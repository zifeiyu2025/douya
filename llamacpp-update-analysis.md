# llama.cpp 最新更新对 douya 的影响分析

**分析对象**：`D:\AI\llama.cpp` 最新提交 `680a9ae63`（语义化版本引入）
**版本跨度**：douya 当前适配 b10355（代码注释引用）；用户日志实际用 b10373；上游最新 tag **b10380**（约 25 个提交差）
**结论**：**douya 当前下载正则对 b10380 全部有效，无需紧急改动**；但上游引入了语义化版本，处于过渡期，建议前瞻放宽正则以防未来失效。

---

## 一、发布包命名：当前未变，仍是 `b` 格式 ✅

`release.yml` 通过 `get-tag-name` action 解析 tag：

```
# .github/actions/get-tag-name/action.yml
BUILD_NUMBER="$(git rev-list --count HEAD)"
echo "name=b${BUILD_NUMBER}" >> $GITHUB_OUTPUT   # 仍是 b10380
```

所以活跃发布包名仍是 `llama-b10380-bin-win-*-x64.zip`，douya 正则 `^llama-b\d+-bin-win-...` **完全匹配**。

**关键事实**：活跃发布流程（`release.yml`）**尚未**切换到语义版本。`make-release.yml` 虽已新增，但目前 `release.yml` 仍是实际产出 release 的工作流。

---

## 二、语义化版本：过渡期风险 ⚠️

| 变更 | 内容 | 影响 |
|------|------|------|
| CMakeLists 版本 | `LLAMA_VERSION = 0.1.0`（MAJOR=0/MINOR=1/PATCH=0） | 内部版本号 |
| 新增 `make-release.yml` | 用 `make-release-checks.sh` 生成 `v0.1.0` tag 并创建 | **未来可能替换** `release.yml` |
| `make-release-checks.sh` | `VERSION="v${MAJOR}.${MINOR}.${PATCH}"` → `v0.1.0` | 一旦启用，包名变成 `llama-v0.1.0-bin-win-...` |

**风险点**：douya 用 `https://api.github.com/repos/ggml-org/llama.cpp/releases/latest`（`backend_download.go:28`）**自动跟随最新 tag**。如果上游把 `release.yml` 改为语义版本 tag，`get-tag-name` 也改输出 `v0.1.0`，则 GitHub latest 会指向 `v0.1.0`，届时 douya 所有 `^llama-b\d+` 正则**立即全部失效**，所有后端下载失败。

**建议（预防性）**：前瞻放宽 `ReleaseAssetRegex`，同时匹配 `b\d+` 与 `v\d+\.\d+\.\d+` 前缀。向后兼容，当前 b10380 仍正常。

---

## 三、DLL 文件名：不受影响 ✅

语义版本提交提到 "include libmtmd in output so show its semversioned"。但 mtmd 的 `CMakeLists.txt` 仅设置 `VERSION`/`SOVERSION`（影响版本资源，不影响 Windows 文件名）：

```cmake
set_target_properties(mtmd PROPERTIES VERSION ${LLAMA_VERSION_BASE} SOVERSION ${LLAMA_VERSION_MAJOR})
```

**Windows (MSVC) 上 DLL 文件名不受 VERSION 影响**，仍是 `mtmd.dll` / `ggml-vulkan.dll` 等。douya 的 `RequiredDLLs`（`mtmdDLL = "mtmd.dll"` 等）校验**全部有效，无需改**。

---

## 四、ROCm 包名：已稳定，douya 已适配 ✅

`#26897 ci: add windows-rocm to check-release` 把 windows-rocm 纳入 release 检查，包名确认为 `llama-b10380-bin-win-rocm-7.14-x64.zip`。douya 此前已修复正则 `^llama-b\d+-bin-win-(rocm-[\d.]+|hip-radeon)-x64\.zip$`（`backend.go:160`），**匹配正确**。

---

## 五、与用户此前痛点直接相关的更新 🔧

| PR | 内容 | 与 douya 用户的关系 |
|----|------|---------------------|
| `#26793 chat: tighten bare function parsing for Qwen models` | 收紧 Qwen 模型裸函数解析 | 用户此前 VLM（Qwen3.5-4B-U）输出乱码，此改进可能间接缓解 Qwen 解析问题（根因仍是 mmproj 在 Vulkan 加载失败） |
| `#25850 vulkan: add TQ2_0 (ternary) support` | Vulkan 新增三元量化 | Vulkan 后端可跑 TQ2_0 量化模型；douya 的 vulkan 安全限制（`gpu-layers capped to 50`）无需改 |
| `#26871 mtmd: support pocket-tts` / `#26640 server: slot save/restore with media inputs` | 多模态（mtmd）增强 | 与用户此前 mmproj 乱码问题相关，新版本多模态可能更稳 |
| `#26111 cuda: wkv7 kernel` / `#26802 CUDA graphs 优化` | CUDA 后端优化 | N 卡用户受益 |
| `#26076 kleidiai` / `#26433 opencl X1E` | ARM/OpenCL 增强 | 边缘设备受益 |

---

## 六、建议的适配改动（按优先级）

1. **【建议实施】前瞻放宽 `ReleaseAssetRegex`**：所有后端正则增加 `(?:b\d+|v\d+\.\d+\.\d+)` 前缀分支，防止上游切换语义版本后下载全失效。向后兼容。
2. **【可选】更新 `b10355` 注释**：`server_args.go:387`、`server.go:250`、`client.go:493` 注释引用 b10355（draft-dspark、load-mode、spec_decode 计数器），内容仍准确，可不改；如想保持时效可更新为 b10380。
3. **【无需改】`releases/latest` 自动跟随**：用户升到 b10380 会自动下载，无需硬编码版本。

---

## 七、当前 douya 正则全景（确认无需紧急改）

| 后端 | 正则 | b10380 状态 |
|------|------|------------|
| CUDA | `^llama-b\d+-bin-win-cuda-1[23]\.\d+-x64\.zip$` | ✅ 匹配 |
| HIP/ROCm | `^llama-b\d+-bin-win-(rocm-[\d.]+|hip-radeon)-x64\.zip$` | ✅ 匹配 |
| SYCL | `^llama-b\d+-bin-win-sycl-x64\.zip$` | ✅ 匹配 |
| Vulkan | `^llama-b\d+-bin-win-vulkan-x64\.zip$` | ✅ 匹配 |
| OpenVINO | `^llama-b\d+-bin-win-openvino-[\d.]+-x64\.zip$` | ✅ 匹配 |
| CPU | `^llama-b\d+-bin-win-cpu-x64\.zip$` | ✅ 匹配 |
