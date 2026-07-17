import { describe, it, expect } from 'vitest'
import { shouldReloadConfigOnModelChange, matchModelRef } from '../stores/settings'

describe('shouldReloadConfigOnModelChange', () => {
  it('returns true when model changes', () => {
    expect(shouldReloadConfigOnModelChange('Qwen3.5-9B', 'Gemma-4-E4B')).toBe(true)
  })

  it('returns false when model is the same', () => {
    expect(shouldReloadConfigOnModelChange('Qwen3.5-9B', 'Qwen3.5-9B')).toBe(false)
  })

  it('returns true when old model is empty', () => {
    expect(shouldReloadConfigOnModelChange('', 'Gemma-4-E4B')).toBe(true)
  })

  it('returns false when new model is empty', () => {
    expect(shouldReloadConfigOnModelChange('Qwen3.5-9B', '')).toBe(false)
  })
})

describe('matchModelRef', () => {
  const refs = {
    'qwen3.5-9b': { name: 'Qwen3.5U-9B' },
    'gemma-4-e4b': { name: 'Gemma4-E4B' },
    'qwen3.5-9b-deepseek': { name: 'DeepSeek-V4-Flash' },
    'qwen3.5-9b-glm': { name: 'GLM5.1-Distill' }
  }

  it('matches Qwen3.5U-9B model name', () => {
    const result = matchModelRef('Qwen3.5U-9B', refs)
    expect(result).not.toBeNull()
    expect(result!.name).toBe('Qwen3.5U-9B')
  })

  it('matches Gemma-4-E4B model name', () => {
    const result = matchModelRef('Gemma-4-E4B', refs)
    expect(result).not.toBeNull()
    expect(result!.name).toBe('Gemma4-E4B')
  })

  it('matches longer key first (deepseek over qwen3.5-9b)', () => {
    const result = matchModelRef('Qwen3.5-9B-DeepSeek-V4-Flash', refs)
    expect(result).not.toBeNull()
    expect(result!.name).toBe('DeepSeek-V4-Flash')
  })

  it('returns null for empty model name', () => {
    expect(matchModelRef('', refs)).toBeNull()
  })

  it('returns null for unknown model', () => {
    expect(matchModelRef('Unknown-Model', refs)).toBeNull()
  })

  it('matches model name from model path', () => {
    const result = matchModelRef('models/Qwen3.5-9B-U-Q4_K_M/Qwen3.5U-9B-Q4_K_M.gguf', refs)
    expect(result).not.toBeNull()
    expect(result!.name).toBe('Qwen3.5U-9B')
  })
})
