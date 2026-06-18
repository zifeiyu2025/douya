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
})
