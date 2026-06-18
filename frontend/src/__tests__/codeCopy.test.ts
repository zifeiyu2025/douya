/**
 * codeCopy 事件委托测试
 *
 * 验证事件委托模式：
 * - 只在容器绑定一次 click 监听器
 * - 动态新增的代码块按钮也能响应（无需重新绑定）
 * - 点击非按钮区域不触发复制
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setupCodeCopyDelegation } from '../utils/codeCopy'

describe('setupCodeCopyDelegation', () => {
  let container: HTMLElement
  let cleanup: (() => void) | null
  let writeTextSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    // mock navigator.clipboard
    writeTextSpy = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: writeTextSpy },
      configurable: true,
    })
  })

  afterEach(() => {
    cleanup?.()
    cleanup = null
    container.remove()
    vi.restoreAllMocks()
  })

  // RED-8：事件委托——动态新增的按钮也能响应
  it('动态新增的代码块按钮点击时触发复制', async () => {
    cleanup = setupCodeCopyDelegation(container)

    // 动态新增一个代码块（模拟流式渲染新增内容）
    const pre = document.createElement('pre')
    const code = document.createElement('code')
    code.textContent = 'console.log("hello")'
    const btn = document.createElement('button')
    btn.className = 'code-copy-btn'
    btn.textContent = '复制'
    pre.appendChild(btn)
    pre.appendChild(code)
    container.appendChild(pre)

    // 点击按钮
    btn.click()
    await new Promise((r) => setTimeout(r, 0))

    expect(writeTextSpy).toHaveBeenCalledWith('console.log("hello")')
  })

  it('点击非按钮区域不触发复制', async () => {
    cleanup = setupCodeCopyDelegation(container)

    const pre = document.createElement('pre')
    const code = document.createElement('code')
    code.textContent = 'some code'
    pre.appendChild(code)
    container.appendChild(pre)

    // 点击 code 元素（非按钮）
    code.click()
    await new Promise((r) => setTimeout(r, 0))

    expect(writeTextSpy).not.toHaveBeenCalled()
  })

  it('cleanup后移除事件监听', async () => {
    cleanup = setupCodeCopyDelegation(container)
    cleanup()
    cleanup = null

    const pre = document.createElement('pre')
    const code = document.createElement('code')
    code.textContent = 'test'
    const btn = document.createElement('button')
    btn.className = 'code-copy-btn'
    pre.appendChild(btn)
    pre.appendChild(code)
    container.appendChild(pre)

    btn.click()
    await new Promise((r) => setTimeout(r, 0))

    expect(writeTextSpy).not.toHaveBeenCalled()
  })

  it('多个代码块按钮都能响应', async () => {
    cleanup = setupCodeCopyDelegation(container)

    for (let i = 0; i < 3; i++) {
      const pre = document.createElement('pre')
      const code = document.createElement('code')
      code.textContent = `code${i}`
      const btn = document.createElement('button')
      btn.className = 'code-copy-btn'
      pre.appendChild(btn)
      pre.appendChild(code)
      container.appendChild(pre)
    }

    const btns = container.querySelectorAll('.code-copy-btn')
    ;(btns[0] as HTMLElement).click()
    ;(btns[2] as HTMLElement).click()
    await new Promise((r) => setTimeout(r, 0))

    expect(writeTextSpy).toHaveBeenCalledTimes(2)
    expect(writeTextSpy).toHaveBeenNthCalledWith(1, 'code0')
    expect(writeTextSpy).toHaveBeenNthCalledWith(2, 'code2')
  })
})
