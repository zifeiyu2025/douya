# Vulkan 日志分析报告

**日志来源**：`C:\Users\cc164\Downloads\Douya-v0.12.2-win64\`（已解压的发布版 v0.12.2，非开发机）
**日志时间**：2026-08-12 19:02 ~ 20:14
**结论一句话**：Vulkan 后端最终已成功安装并运行；日志里「识别到显存为 0」是兜底检测的**正常设计**，而前期「用不了 Vulkan」的真实根因是**从 GitHub 下载后端包时网络失败**（连接超时 / 协议错误），属于网络环境问题，不是代码缺陷。

---

## 一、为什么「识别到显存为 0」——这是设计预期，不是 bug

日志每次启动都出现（如第 8-9 行）：

```
[system] Vulkan device detected (fallback, vendor unknown)
vendor":"vulkan","gpu":"Vulkan Device","vram_mb":0
```

原因对应代码 `internal/system/hardware.go` 的检测顺序与 `detectVulkanDevice` 兜底逻辑：

1. 检测顺序为 NVIDIA → AMD → Intel → Vulkan 依次兜底。
2. 这台机器：`nvcuda.dll` 不存在（无 NVIDIA）、无 AMD 驱动、无 Intel 驱动（日志第 4-7 行），所以三个品牌都没命中。
3. 最后触发 Vulkan 兜底：仅检查 `C:\Windows\System32\vulkan-1.dll` 是否存在。只要存在，就把 `HasGPU=true`、`GPUVendor="vulkan"`、`GPUName="Vulkan Device"`，**并显式把 `GPUVRAMMB=0`**。

单元测试用例 `hardware_test.go` 的注释直接说明这一点：

> `期望 GPUVRAMMB=0（Vulkan 无显存信息）`

**也就是说**：Vulkan 兜底探测本身**不去查询具体厂商、型号和显存大小**，它只是「系统里有可用的 Vulkan 运行时」的一个弱信号。`vram_mb=0` 是有意为之——真正的显存与设备信息会交给 llama.cpp 运行时在加载模型时自动探测。所以「显存 0」完全正常，不用修。

---

## 二、为什么「用不了 Vulkan」——根因是下载网络失败

auto 模式下检测到 vulkan 后，应用需要去 GitHub 下载官方预编译后端包（llama.cpp 自某版本起改为**模块化后端**：Vulkan 主包必须先有 CPU 基础包作底盘）。

### 完整时间线

| 时间 | 事件 | 说明 |
|------|------|------|
| 19:02:30 | `[startup] 后端安装失败 ... 未找到 Vulkan 后端 zip 包` | 首次启动，vulkan 包尚未下载 |
| 19:02:35 | 用户弹窗选择「从 GitHub 下载后端」 | 同意下载 |
| 19:02:36 → 19:15:18 | 下载 CPU 基础包 `llama-b10373-bin-win-cpu-x64.zip`（18.4MB）**耗时约 13 分钟** | 网络极慢，但仍成功 |
| 19:15:18 → 19:22 | 开始下载 Vulkan 主包 `llama-b10373-bin-win-vulkan-x64.zip`（34.2MB） | 此后 19:22 启动显示 vulkan 仍未装好，说明该次下载未完成/被中断 |
| 19:42:45 ~ 19:45:28 | Vulkan 主包下载**连续失败 3 轮 × 3 次** | 每次都报 `dial tcp 20.205.243.166:443 ... did not properly respond`（连接超时） |
| 19:45:44 ~ 20:01:28 | 又一轮下载，前 1 次超时，第 2 次报 `stream error ... PROTOCOL_ERROR` | HTTP/2 协议层错误，重试 3 次放弃 |
| **20:06:11** | `[backend] 开始解压后端 zip 包` → `[backend] 后端安装完成` → `runtime/vulkan/llama-server.exe` | **下载解压成功，Vulkan 后端安装完成** |
| 20:06:11 / 20:11:32 | `[server-config] Vulkan backend safety limit: ctx-size capped to 8192`；`gpu-layers capped to 50` | **Vulkan 后端实际已生效并运行**（安全限制只对 Vulkan 后端触发） |

### 失败报错原文（关键证据）

```
下载请求失败: Get ".../llama-b10373-bin-win-vulkan-x64.zip":
dial tcp 20.205.243.166:443: connectex: A connection attempt failed
because the connected party did not properly respond after a period of time
```

```
stream error: stream ID 1; PROTOCOL_ERROR; received from peer
```

`20.205.243.166` 是 GitHub 的 CDN 地址。连接超时 + HTTP/2 `PROTOCOL_ERROR`，再结合 CPU 包花了 13 分钟才下完，典型的**网络到 GitHub 不稳定 / 受限（GFW 或代理问题）**特征，与应用代码无关。应用侧已正确实现了「重试 3 次 → 提示失败」的降级逻辑。

---

## 三、最终状态：Vulkan 后端是**正常工作**的

20:06 之后多次启动都成功加载 Vulkan 后端，仅对 Vulkan 触发的两条安全限制足以证明后端已被选中并运行：

```
[server-config] Vulkan backend safety limit: gpu-layers capped to 50
[server-config] Vulkan backend safety limit: ctx-size capped to 8192
```

模型 `Qwen3.5-4B-U` 也成功加载（text-only 模式，mmproj 视觉组件加载失败后自动降级，属正常兼容逻辑）。

---

## 四、给用户与产品的建议

1. **「显存 0」无需处理**：这是 Vulkan 兜底探测的设计行为，真正显存由 llama.cpp 运行时在推理时探测，不影响使用。
2. **确保能访问 GitHub 才能下载后端包**：首次使用 vulkan 后端需要联网从 GitHub 拉取 CPU 基础包 + Vulkan 主包（约 52MB）。若网络访问 GitHub 困难：
   - 配置系统代理 / 让豆芽走代理；
   - 或在可联网环境手动下载 `llama-b10373-bin-win-cpu-x64.zip` 与 `llama-b10373-bin-win-vulkan-x64.zip`，解压到 `runtime/cpu` 与 `runtime/vulkan` 目录。
3. **（可选产品优化，非当前 bug）**：
   - 下载失败时给出更明确的「网络不可达 GitHub，请检查代理/镜像」提示，而非仅 `PROTOCOL_ERROR`；
   - 支持可配置的 GitHub 镜像源，降低国内用户下载失败率；
   - 考虑提升 Vulkan 兜底检测的 VRAM 查询（调用 `vulkaninfo` 等），让 UI 能显示显存，但属增强特性。

---

## 附：关键日志行索引

- 第 8-9 行：Vulkan 兜底 + `vram_mb:0`
- 第 12 行：首次「未找到 Vulkan 后端 zip 包」
- 第 19-23 行：CPU 基础包下载成功
- 第 256-258、277-291、312-326、346-347 行：Vulkan 主包多次下载失败（超时 / PROTOCOL_ERROR）
- 第 364-365 行：Vulkan 包最终下载解压成功
- 第 379、426-427 行：Vulkan 后端安全限制生效（证明后端运行）
