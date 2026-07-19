import { describe, it, expect } from 'vitest'
import { decideSpecAdviceNotification } from './specAdvice'
import type { SpecAdvice } from '../types/chat'

// 测试用的 SpecAdvice 数据（Eagle3 场景）
const eagle3Advice: SpecAdvice = {
  sidecar: 'eagle3',
  desc: 'Eagle3',
  download_url: 'https://hf-mirror.com/unsloth/eagle3-models/tree/main',
  reason: '模型支持 Eagle3 推测解码，但未配置 draft 模型'
}

// 测试用的 SpecAdvice 数据（DFlash 场景）
const dflashAdvice: SpecAdvice = {
  sidecar: 'dflash',
  desc: 'DFlash',
  download_url: 'https://hf-mirror.com/search?search_keyword=qwen3.6+dflash',
  reason: '模型支持 DFlash 推测解码，但未配置 draft 模型'
}

describe('decideSpecAdviceNotification - 推测解码通知决策', () => {
  it('所有条件满足：模型首次就绪 + 有建议 + 开关开 + 未 dismiss → 应弹通知', () => {
    const result = decideSpecAdviceNotification({
      prevReady: false,
      currReady: true,
      specAdvice: eagle3Advice,
      adviceEnabled: true,
      dismissedKeys: []
    })
    expect(result.shouldShow).toBe(true)
    expect(result.dismissKey).toBe('spec_advice:eagle3')
  })

  it('开关关闭 → 不显示（尊重用户选择）', () => {
    const result = decideSpecAdviceNotification({
      prevReady: false,
      currReady: true,
      specAdvice: eagle3Advice,
      adviceEnabled: false,
      dismissedKeys: []
    })
    expect(result.shouldShow).toBe(false)
    expect(result.dismissKey).toBe('')
  })

  it('模型未就绪 → 不显示', () => {
    const result = decideSpecAdviceNotification({
      prevReady: false,
      currReady: false,
      specAdvice: eagle3Advice,
      adviceEnabled: true,
      dismissedKeys: []
    })
    expect(result.shouldShow).toBe(false)
  })

  it('模型从就绪→就绪（非边沿触发）→ 不显示，避免重复打扰', () => {
    const result = decideSpecAdviceNotification({
      prevReady: true,
      currReady: true,
      specAdvice: eagle3Advice,
      adviceEnabled: true,
      dismissedKeys: []
    })
    expect(result.shouldShow).toBe(false)
  })

  it('specAdvice 为 null → 不显示', () => {
    const result = decideSpecAdviceNotification({
      prevReady: false,
      currReady: true,
      specAdvice: null,
      adviceEnabled: true,
      dismissedKeys: []
    })
    expect(result.shouldShow).toBe(false)
  })

  it('已 dismiss 过同 sidecar 类型 → 不再弹', () => {
    const result = decideSpecAdviceNotification({
      prevReady: false,
      currReady: true,
      specAdvice: eagle3Advice,
      adviceEnabled: true,
      dismissedKeys: ['spec_advice:eagle3']
    })
    expect(result.shouldShow).toBe(false)
  })

  it('dismiss 过 eagle3 但当前是 dflash → 应弹（不同 sidecar 独立计数）', () => {
    const result = decideSpecAdviceNotification({
      prevReady: false,
      currReady: true,
      specAdvice: dflashAdvice,
      adviceEnabled: true,
      dismissedKeys: ['spec_advice:eagle3']
    })
    expect(result.shouldShow).toBe(true)
    expect(result.dismissKey).toBe('spec_advice:dflash')
  })
})
