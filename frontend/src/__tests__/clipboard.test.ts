/**
 * clipboard 工具测试
 *
 * 验证 copyText 的两条路径：
 * - 安全上下文下优先使用 navigator.clipboard.writeText
 * - clipboard 不可用/失败时降级为 execCommand('copy')
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { copyText } from '../utils/clipboard'

describe('copyText', () => {
  let writeTextSpy: ReturnType<typeof vi.fn>
  let execCommandSpy: ReturnType<typeof vi.fn>

  beforeEach(() => {
    writeTextSpy = vi.fn()
    // jsdom 未实现 execCommand，直接以 mock 函数挂载
    execCommandSpy = vi.fn()
    ;(document as unknown as { execCommand: unknown }).execCommand = execCommandSpy
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('安全上下文下使用 navigator.clipboard 并返回 true', async () => {
    writeTextSpy.mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: writeTextSpy },
      configurable: true
    })
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })

    await expect(copyText('hello')).resolves.toBe(true)
    expect(writeTextSpy).toHaveBeenCalledWith('hello')
    expect(execCommandSpy).not.toHaveBeenCalled()
  })

  it('clipboard.writeText 失败时降级为 execCommand', async () => {
    writeTextSpy.mockRejectedValue(new Error('denied'))
    execCommandSpy.mockReturnValue(true)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: writeTextSpy },
      configurable: true
    })
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })

    await expect(copyText('hello')).resolves.toBe(true)
    expect(execCommandSpy).toHaveBeenCalledWith('copy')
  })

  it('非安全上下文（无 clipboard）时降级为 execCommand', async () => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
    Object.defineProperty(window, 'isSecureContext', { value: false, configurable: true })
    execCommandSpy.mockReturnValue(true)

    await expect(copyText('hello')).resolves.toBe(true)
    expect(execCommandSpy).toHaveBeenCalledWith('copy')
  })

  it('execCommand 也失败时返回 false 而不抛出', async () => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
    Object.defineProperty(window, 'isSecureContext', { value: false, configurable: true })
    execCommandSpy.mockReturnValue(false)

    await expect(copyText('hello')).resolves.toBe(false)
  })
})
