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
      // 思考模式（客户端已细分到具体版本卡时）：Qwen3-Thinking-2507 官方 temperature=0.6
      // 注意：Qwen3-30B-A3B/14B/8B 现匹配各自细化卡片，思考温度均为 0.6
      expect(result?.raw_thinking?.temperature).toBe(0.6)
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

  it('匹配 Qwen3 具体版本（8B/14B/30B-A3B）细化卡片', () => {
    // Qwen3 各具体版本应匹配各自的独立卡片，而非 qwen3 兜底
    const cases: Array<[string, string]> = [
      ['Qwen3-8B-Instruct', 'Qwen3-8B'],
      ['Qwen3-8B-Instruct-2507-GGUF', 'Qwen3-8B'],
      ['Qwen3-14B-Instruct', 'Qwen3-14B'],
      ['Qwen3-30B-A3B-Instruct-2507', 'Qwen3-30B-A3B']
    ]
    for (const [name, expected] of cases) {
      const result = matchModelRef(name, MODEL_REFS)
      expect(result, `${name} should match ${expected}`).not.toBeNull()
      expect(result?.name).toBe(expected)
      // Qwen3 官方：非思考 t=0.7/top_p=0.8/top_k=20，思考 t=0.6/top_p=0.95（Qwen3-Thinking-2507）
      expect(result?.raw.temperature).toBe(0.7)
      expect(result?.raw.top_p).toBe(0.8)
      expect(result?.raw.top_k).toBe(20)
      expect(result?.raw_thinking?.temperature).toBe(0.6)
      expect(result?.raw_thinking?.top_p).toBe(0.95)
    }
    // 未细分的未来版本仍兜底到 qwen3 系列，而非被具体版本误匹配
    const t0 = matchModelRef('Qwen3-20B-SomeFutureVariant', MODEL_REFS)
    expect(t0).not.toBeNull()
    expect(t0?.name.startsWith('Qwen3')).toBe(true)
  })

  it('匹配 Qwen3.8 具体版本（9B）与兜底', () => {
    // Qwen3.8-9B 应匹配独立卡片
    const r9 = matchModelRef('Qwen3.8-9B-Instruct', MODEL_REFS)
    expect(r9).not.toBeNull()
    expect(r9?.name).toBe('Qwen3.8-9B')
    expect(r9?.raw.temperature).toBe(0.7)
    expect(r9?.raw_thinking?.temperature).toBe(1.0)
    // 未明确的 Qwen3.8 变体兜底到系列卡片
    const rFallback = matchModelRef('Qwen3.8-SomeVariant', MODEL_REFS)
    expect(rFallback).not.toBeNull()
    expect(rFallback?.name).toBe('Qwen3.8 系列')
    // 关键：不被 qwen3-8b（8B）误匹配
    expect(rFallback?.name).not.toBe('Qwen3-8B')
  })

  it('匹配 Qwen3 主力系列（Qwen3-30B-A3B / Qwen3-14B / Qwen3-8B 等）', () => {
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

  it('匹配 MiniCPM5 端侧思考模型及变体', () => {
    const result = matchModelRef('MiniCPM5-1B-Claude-Opus-Fable5-V2-Thinking-F16', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.name).toBe('MiniCPM5')
    expect(result?.raw.top_k).toBe(20)
    // 思考模式参数也应存在
    expect(result?.raw_thinking).toBeDefined()
    expect(result?.raw_thinking?.temperature).toBe(1.0)
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

  // ===== 后端已支持但前端此前缺失的模型匹配测试 =====
  it('匹配 Mistral Small 3', () => {
    const result = matchModelRef('Mistral-Small-3.1-24B-Instruct', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.name).toBe('Mistral Small 3')
    expect(result?.raw_thinking).toBeDefined()
  })

  it('匹配 Mistral 4', () => {
    const result = matchModelRef('Mistral-4-24B-Instruct', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.name).toBe('Mistral 4')
  })

  it('匹配 ERNIE 4.5 Dense 和 MoE 变体', () => {
    const dense = matchModelRef('ERNIE-4.5-8B-Chat', MODEL_REFS)
    expect(dense).not.toBeNull()
    expect(dense?.name).toBe('ERNIE 4.5')
    const moe = matchModelRef('ERNIE-4.5-MoE-300B', MODEL_REFS)
    expect(moe).not.toBeNull()
    expect(moe?.name).toBe('ERNIE 4.5 MoE')
  })

  it('匹配 Hunyuan MoE 和 Dense 变体', () => {
    const moe = matchModelRef('Hunyuan-MoE-389B', MODEL_REFS)
    expect(moe).not.toBeNull()
    expect(moe?.name).toBe('Hunyuan MoE')
    const dense = matchModelRef('Hunyuan-Dense-7B', MODEL_REFS)
    expect(dense).not.toBeNull()
    expect(dense?.name).toBe('Hunyuan Dense')
  })

  it('匹配 SmolLM3 端侧思考模型', () => {
    const result = matchModelRef('SmolLM3-3B', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.raw_thinking?.temperature).toBe(1.0)
  })

  it('匹配 Step 3.5（含 MTP）', () => {
    const result = matchModelRef('Step3.5-8B', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.name).toBe('Step 3.5')
  })

  it('匹配 DeepSeek 3 系列（Reasoning 模式）', () => {
    const result = matchModelRef('DeepSeek3.2-16B', MODEL_REFS)
    expect(result).not.toBeNull()
    expect(result?.name).toBe('DeepSeek 3 / 3.2 / 32')
  })

  it('匹配 DeepSeek 4 / V4（Reasoning 模式）', () => {
    const r1 = matchModelRef('DeepSeek4-16B', MODEL_REFS)
    expect(r1).not.toBeNull()
    expect(r1?.name).toBe('DeepSeek 4')
    const r2 = matchModelRef('DeepSeek-V4-16B', MODEL_REFS)
    expect(r2).not.toBeNull()
    expect(r2?.name).toBe('DeepSeek V4')
  })

  it('匹配 MiniMax M2 / Kimi Linear / Arcee / Dots1 / Dream / SmallThinker', () => {
    const cases: Array<[string, string]> = [
      ['MiniMax-M2-230B', 'MiniMax M2'],
      ['Kimi-Linear-20B', 'Kimi Linear'],
      ['Arcee-8B', 'Arcee'],
      ['Dots1-14B', 'Dots1'],
      ['Dream-7B', 'Dream'],
      ['SmallThinker-3B', 'SmallThinker']
    ]
    for (const [name, expected] of cases) {
      const result = matchModelRef(name, MODEL_REFS)
      expect(result, `${name} should match`).not.toBeNull()
      expect(result?.name).toBe(expected)
    }
  })
})
