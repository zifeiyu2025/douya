# 聊天回复乱码根因分析

**现象**：用户发送「你好」，AI 回复乱码文本（`anA and7，由"、提供的、`、`AI、实时( ' 、'不"。——性（君`）

**结论一句话**：**模型问题，不是代码 bug**。`Qwen3.5-4B-U` 是多模态视觉语言模型（VLM），其 mmproj（视觉投影器）在 Vulkan 后端下加载失败，降级为纯文本模式后模型的 tokenizer/模板与纯文本模式不兼容，导致输出乱码。

---

## 日志证据链

### 1. 模型是多模态 VLM（非纯文本模型）

```
第 385 行: "image":true,"audio":false,"text":true,"reasoning":true
第 388 行: mmproj="C:\...\models\mmproj-Qwen3.5-4B-U-F16.gguf"
```

`Qwen3.5-4B-U` 的能力检测：`image: true` — 这是一个**视觉语言模型**，需要配合 mmproj 投影器文件才能正常工作。

### 2. mmproj 加载失败，每次都降级为 text-only

```
第 388 行: [preset] removing mmproj from preset due to loading failure, will retry in text-only mode
第 394 行: [server] model loaded successfully without mmproj (text-only mode)
第 436 行: [preset] removing mmproj from preset due to loading failure ... (重启后再次失败)
第 442 行: [server] model loaded successfully without mmproj (text-only mode) (重启后再次)
```

**每次启动** mmproj 都加载失败 → 自动移除 mmproj → 以纯文本模式重试加载模型。

### 3. 模型只输出"思考"内容，正文始终为空

```
第 396 行: [stream] thinking completed but content is empty (finish_reason=stop, thinking_len=68)
第 398 行: [stream] thinking completed but content is empty (finish_reason=stop, thinking_len=7)
第 444 行: [stream] thinking completed but content is empty (finish_reason=stop, thinking_len=38)
第 447 行: [stream] thinking completed but content is empty (finish_reason=stop, thinking_len=31)
```

**4 次对话全部如此**：`finish_reason=stop`（正常结束），但 `FullContent.Len() == 0`（正文为空），只有 `FullThinking` 有内容。

### 4. 用户看到的"乱码"就是损坏的思考内容

截图中的两种展示：
- 第一条回复：`anA and7，由"、提供的、` 出现在「已思考(用时0.6秒)」折叠区内 → 这是 **thinking 内容**
- 第二条回复：`AI、实时( ' 、'不"。——性（君` 作为正文显示 → 可能是前端将空的 content + 非空的 thinking 做了兼容展示

---

## 根因分析

### 为什么 mmproj 加载失败？

可能原因（按概率排序）：

1. **Vulkan 后端不支持 mmproj 多模态**：llama.cpp 的 Vulkan 后端对 CLIP/Vision 投影器的支持可能不完整或存在已知限制
2. **显存不足**：Vulkan 后端安全限制 `gpu-layers capped to 50` + `ctx-size capped to 8192`（日志第 426-427 行），mmproj 额外需要数百 MB 显存，Vulkan 兜底检测又报 vram_mb=0（无法做准确预算）
3. **mmproj 文件与主模型版本不匹配**：mmproj-Qwen3.5-4B-U-F16.gguf 可能是基于不同 llama.cpp 版本编译的，与当前 b10373 的 Vulkan 后端不兼容

### 为什么 text-only 模式下输出乱码？

`Qwen3.5-4B-U` 作为 VLM，其训练数据包含大量 `<|vision_start|>` 等特殊标记，chat template 也可能包含视觉 token 占位符。在 text-only 模式下：

1. **Chat template 不匹配**：模板可能期望有图像输入，缺失时导致 prompt 构建异常
2. **Tokenizer 输出异常 token**：模型的分词器可能将中文文本编码成了视觉相关的子词 token，解码后变成乱码
3. **Thinking/Reasoning token 损坏**：Qwen3.5 的思考格式（`<think＞...＜/think＞`）可能在缺少视觉组件时无法正确解析，导致思考内容本身就是乱码

---

## 解决方案（按推荐优先级）

### 方案 1：换用纯文本模型（推荐，立竿见影）

Qwen3.5-4B-U 是多模态模型。如果不需要图像理解功能，换用纯文本模型即可彻底解决：

- **Qwen2.5-7B-Instruct**（纯文本，成熟稳定）
- **Qwen2.5-3B-Instruct**（更小，适合低显存）
- **Llama 3.1/3.2 系列**（Meta 出品，纯文本）

这些模型没有 mmproj 依赖，不存在降级问题。

### 方案 2：让 mmproj 正常加载（需要非 Vulkan 后端）

如果必须用 Qwen3.5-4B-U 的多模态能力：
- 使用 NVIDIA CUDA 后端（需要 N 卡）
- 或等 llama.cpp 上游修复 Vulkan 后端的 mmproj 支持问题

### 方案 3（代码层面可做的防御优化）

虽然根因是模型/后端兼容性问题，但代码可以做更好的提示，避免用户困惑：

1. **mmproj 加载失败时给出明确警告**：「检测到多模态模型 X 的视觉组件加载失败，当前后端可能不完全支持多模态推理，建议换用纯文本模型或使用 CUDA 后端」
2. **text-only 模式下 VLM 输出为空时给出提示**：「模型未产生有效回复（可能是多模态模型在纯文本模式下的兼容性问题）」
3. **thinking 内容为空时不显示乱码的 thinking 区域**

---

## 总结

| 项目 | 说明 |
|------|------|
| **是否 douya 代码 bug** | 否 |
| **根因** | Qwen3.5-4B-U（VLM）的 mmproj 在 Vulkan 后端加载失败 → text-only 模式下 tokenizer/template 不兼容 → 输出乱码 |
| **为什么每次都是乱码** | 4 次请求全部 `content 为空 + thinking 有内容但乱码`，100% 复现 |
| **最快解决方案** | 换用纯文本模型（Qwen2.5 / Llama 等） |
| **可选代码优化** | mmproj 失败时给用户友好提示 + 空内容时不在 UI 显示乱码 thinking |
