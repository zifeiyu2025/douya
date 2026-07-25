/**
 * modelRefs 常量测试
 */
import { describe, it, expect } from 'vitest'
import { MODEL_REFS } from './modelRefs'
import { matchModelRef } from '../stores/settings'

describe('MODEL_REFS', () => {
  it('包含至少一个模型配置', () => {
    expect(Object.keys(MODEL_REFS).length).toBeGreaterThan(0)
  })

  it('每个配置都有 name 和 params 字段', () => {
    for (const [key, ref] of Object.entries(MODEL_REFS)) {
      expect(ref.name, `${key} should have name`).toBeTruthy()
      expect(ref.params, `${key} should have params`).toBeInstanceOf(Array)
      expect(ref.params.length, `${key} params should not be empty`).toBeGreaterThan(0)
      expect(ref.raw, `${key} should have raw`).toBeDefined()
    }
  })
})

describe('matchModelRef', () => {
  it('匹配已知模型名', () => {
    // 获取第一个 key 用于测试
    const firstKey = Object.keys(MODEL_REFS)[0]
    const result = matchModelRef(firstKey, MODEL_REFS)
    expect(result).toBeDefined()
    expect(result?.name).toBeTruthy()
  })

  it('未知模型返回 null', () => {
    const result = matchModelRef('nonexistent-model-xyz', MODEL_REFS)
    expect(result).toBeNull()
  })

  it('匹配 Qwythos 系列模型名', () => {
    // Qwythos-9B-Claude-Mythos-5-1M-GGUF，关键词 qwythos 应匹配整个系列
    const result = matchModelRef('Qwythos-9B-Claude-Mythos-5-1M', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.name).toBeTruthy()
    // 推荐参数应包含 temperature=0.6（Qwen3.5 官方思考模式推荐）
    expect(result?.raw.temperature).toBe(0.6)
    expect(result?.raw.top_p).toBe(0.95)
    expect(result?.raw.top_k).toBe(20)
    expect(result?.raw.repeat_penalty).toBe(1.05)
  })

  // ============================================================
  // 主流开源模型卡片覆盖测试（Tier 1）
  // 验证主流模型名能匹配到卡片，且参数符合官方推荐
  // ============================================================

  it('匹配 Qwen3 主力系列（Qwen3-30B-A3B / Qwen3-14B / Qwen3-8B 等）', () => {
    // 各种 Qwen3 主力模型名都应匹配到 qwen3 卡片
    const testNames = ['Qwen3-30B-A3B-Instruct-2507-GGUF', 'Qwen3-14B-Instruct-2507', 'Qwen3-8B']
    for (const name of testNames) {
      const result = matchModelRef(name, MODEL_REFS)
      expect(result, `${name} should match`).not.toBeNull()
      // Qwen3 官方推荐：非思考 temperature=0.7, top_p=0.8, top_k=20
      expect(result?.raw.temperature).toBe(0.7)
      expect(result?.raw.top_p).toBe(0.8)
      expect(result?.raw.top_k).toBe(20)
      // 思考模式：temperature=1.0, top_p=0.95
      expect(result?.raw_thinking?.temperature).toBe(1.0)
      expect(result?.raw_thinking?.top_p).toBe(0.95)
    }
  })

  it('匹配 Qwen3.5 具体版本（4B/9B）', () => {
    // Qwen3.5-4B 系列（含无审查变体）应匹配 qwen3.5-4b，而非 qwen3
    const testNames4B = [
      'Qwen3.5-4B-Instruct',
      'Qwen3.5-4B-Uncensored-HauhauCS-Aggressive-Q6_K.gguf'
    ]
    for (const name of testNames4B) {
      const result = matchModelRef(name, MODEL_REFS)
      expect(result, `${name} should match qwen3.5-4b`).not.toBeNull()
      expect(result?.name).toBe('Qwen3.5-4B')
    }
    // Qwen3.5-9B 系列（含 U 变体）应匹配 qwen3.5-9b
    const testNames9B = ['Qwen3.5-9B-Instruct', 'Qwen3.5U-9B-Q4_K_M.gguf']
    for (const name of testNames9B) {
      const result = matchModelRef(name, MODEL_REFS)
      expect(result, `${name} should match qwen3.5-9b`).not.toBeNull()
      expect(result?.name).toBe('Qwen3.5-9B')
    }
  })

  it('Qwen3.5 未知版本兜底匹配（不误判为 Qwen3）', () => {
    // 未明确标注 4B/9B 的 Qwen3.5 变体应匹配 qwen3.5 兜底卡片，而非 qwen3
    const result = matchModelRef('Qwen3.5-SomeFutureVariant-7B', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.name).toBe('Qwen3.5 系列')
    // 关键断言：不能匹配到 Qwen3 系列
    expect(result?.name).not.toBe('Qwen3 系列')
  })

  it('匹配 Qwen3-Coder 编程专用模型', () => {
    const result = matchModelRef('Qwen3-Coder-30B-A3B-Instruct-GGUF', MODEL_REFS)
    expect(result).not.toBeNull()
    // 编程推荐低温度（与 Qwen2.5-Coder 风格一致）
    expect(result?.raw.temperature).toBeLessThanOrEqual(0.3)
  })

  it('匹配 Qwen3-VL 多模态模型', () => {
    const result = matchModelRef('Qwen3-VL-30B-A3B-Instruct', MODEL_REFS)
    expect(result).not.toBeNull()
    // 参数风格与 Qwen3 主力一致
    expect(result?.raw.top_k).toBe(20)
  })

  it('匹配 Gemma 3 主力系列（12B/4B/1B）', () => {
    const testNames = ['gemma-3-12b-it', 'gemma-3-4b-it', 'gemma-3-1b-it']
    for (const name of testNames) {
      const result = matchModelRef(name, MODEL_REFS)
      expect(result, `${name} should match`).not.toBeNull()
      // Google 官方推荐：temperature=1.0, top_k=64
      expect(result?.raw.temperature).toBe(1.0)
      expect(result?.raw.top_k).toBe(64)
    }
  })

  it('匹配 Gemma 3n 端侧模型', () => {
    const result = matchModelRef('gemma-3n-e4b-it', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.raw.top_k).toBe(64)
  })

  it('匹配 GLM-4 / ChatGLM4 系列', () => {
    const testNames = ['glm-4-9b-chat', 'chatglm4-9b', 'GLM-4-Plus']
    for (const name of testNames) {
      const result = matchModelRef(name, MODEL_REFS)
      expect(result, `${name} should match`).not.toBeNull()
      // GLM-4 官方推荐 top_k=20
      expect(result?.raw.top_k).toBe(20)
    }
  })

  it('匹配 Mistral Nemo 12B', () => {
    const result = matchModelRef('Mistral-Nemo-Instruct-2407', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.raw.top_p).toBe(0.9)
    expect(result?.raw.top_k).toBe(40)
  })

  it('匹配 Mixtral 8x7B MoE', () => {
    const result = matchModelRef('mixtral-8x7b-instruct-v0.1', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.raw.top_k).toBe(40)
  })

  it('匹配 Mixtral 8x22B 大 MoE', () => {
    const result = matchModelRef('mixtral-8x22b-instruct-v0.1', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.raw.top_k).toBe(40)
  })

  it('匹配 Codestral 编程专用模型', () => {
    const result = matchModelRef('Codestral-22B-v0.1', MODEL_REFS)
    expect(result).not.toBeNull()
    // 编程推荐低温度
    expect(result?.raw.temperature).toBeLessThanOrEqual(0.3)
  })

  it('匹配 Command R+ RAG 优化模型', () => {
    const result = matchModelRef('command-r-plus', MODEL_REFS)
    expect(result).not.toBeNull()
    // RAG 推荐低温度
    expect(result?.raw.temperature).toBeLessThanOrEqual(0.4)
  })

  it('匹配 MiniCPM3 端侧国产模型', () => {
    const result = matchModelRef('MiniCPM3-4B', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.raw.top_k).toBe(40)
  })

  it('匹配 Phi-3.5 小模型', () => {
    const result = matchModelRef('Phi-3.5-mini-instruct', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.raw.top_k).toBe(40)
  })

  it('匹配 InternLM2.5 国产主力', () => {
    const result = matchModelRef('internlm2_5-7b-chat', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.raw.top_k).toBe(40)
  })

  it('匹配 Llama 3.2 边缘设备模型（1B/3B）', () => {
    const testNames = ['Llama-3.2-1B-Instruct', 'Llama-3.2-3B-Instruct']
    for (const name of testNames) {
      const result = matchModelRef(name, MODEL_REFS)
      expect(result, `${name} should match`).not.toBeNull()
      // 与 Llama 3.1 参数风格一致
      expect(result?.raw.temperature).toBe(0.6)
      expect(result?.raw.top_p).toBe(0.9)
      expect(result?.raw.top_k).toBe(40)
    }
  })
})
