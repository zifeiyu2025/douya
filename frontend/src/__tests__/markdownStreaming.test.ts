/**
 * markdownStreaming 工具测试
 *
 * 验证流式 Markdown 渲染的优化逻辑：
 * - 内容拆分：按块级边界（空行）拆分为稳定块和不稳定块
 * - 渲染合并：多次快速调用只处理最新内容
 * - 稳定块缓存：已渲染的稳定块不重复渲染
 */
import { describe, it, expect, vi } from 'vitest'
import { splitStableUnstable, createStreamingRenderer } from '../utils/markdownStreaming'

describe('splitStableUnstable', () => {
  // RED-5：按块级边界（空行）拆分
  it('多个段落时按最后一个空行拆分', () => {
    const content = '段落1\n\n段落2\n\n正在生成'
    const { stable, unstable } = splitStableUnstable(content)
    expect(stable).toBe('段落1\n\n段落2\n\n')
    expect(unstable).toBe('正在生成')
  })

  it('无空行边界时全部为不稳定块', () => {
    const content = '正在生成的一段话'
    const { stable, unstable } = splitStableUnstable(content)
    expect(stable).toBe('')
    expect(unstable).toBe('正在生成的一段话')
  })

  it('仅一个空行时拆分为稳定和不稳定', () => {
    const content = '段落1\n\n段落2'
    const { stable, unstable } = splitStableUnstable(content)
    expect(stable).toBe('段落1\n\n')
    expect(unstable).toBe('段落2')
  })

  it('空字符串返回空稳定和空不稳定', () => {
    const { stable, unstable } = splitStableUnstable('')
    expect(stable).toBe('')
    expect(unstable).toBe('')
  })

  it('以空行结尾时全部为稳定块', () => {
    const content = '段落1\n\n段落2\n\n'
    const { stable, unstable } = splitStableUnstable(content)
    expect(stable).toBe('段落1\n\n段落2\n\n')
    expect(unstable).toBe('')
  })
})

describe('createStreamingRenderer', () => {
  // RED-6：渲染合并——多次快速调用只处理最新内容
  it('同步多次调用只渲染最后一次内容', async () => {
    const renderFn = vi.fn(async (c: string) => `<p>${c}</p>`)
    const renderer = createStreamingRenderer(renderFn)

    // 同步连续调用三次（同一 microtask 内）
    const p1 = renderer.render('a')
    const p2 = renderer.render('ab')
    const p3 = renderer.render('abc')
    const results = await Promise.all([p1, p2, p3])

    // renderFn 只被调用一次，参数是最后一次的 'abc'
    expect(renderFn).toHaveBeenCalledTimes(1)
    expect(renderFn).toHaveBeenCalledWith('abc')
    // 三个 promise 返回相同结果
    expect(results[0]).toBe(results[1])
    expect(results[1]).toBe(results[2])
  })

  // RED-7：稳定块缓存——已渲染的稳定块不重复渲染
  it('稳定块命中缓存时只渲染不稳定块', async () => {
    const renderFn = vi.fn(async (c: string) => `<p>${c.replace(/\n/g, '')}</p>`)
    const renderer = createStreamingRenderer(renderFn)

    // 第一次：stable="A\n\n", unstable="B"
    const html1 = await renderer.render('A\n\nB')
    const callsAfterFirst = renderFn.mock.calls.length
    // 第一次需要渲染 stable 和 unstable
    expect(callsAfterFirst).toBe(2)
    expect(html1).toBe('<p>A</p><p>B</p>')

    // 第二次：stable="A\n\n"（相同），unstable="BC"
    const html2 = await renderer.render('A\n\nBC')
    // stable 命中缓存，只渲染 unstable "BC"
    expect(renderFn.mock.calls.length).toBe(callsAfterFirst + 1)
    expect(renderFn.mock.calls[callsAfterFirst][0]).toBe('BC')
    expect(html2).toBe('<p>A</p><p>BC</p>')
  })

  it('稳定块变化时重新渲染stable', async () => {
    const renderFn = vi.fn(async (c: string) => `<p>${c.replace(/\n/g, '')}</p>`)
    const renderer = createStreamingRenderer(renderFn)

    await renderer.render('A\n\nB')
    const callsAfterFirst = renderFn.mock.calls.length

    // stable 变化：A → C
    await renderer.render('C\n\nD')
    // stable 变了，需要重新渲染 stable "C\n\n" 和 unstable "D"
    expect(renderFn.mock.calls.length).toBe(callsAfterFirst + 2)
  })

  it('reset后清除缓存', async () => {
    const renderFn = vi.fn(async (c: string) => `<p>${c.replace(/\n/g, '')}</p>`)
    const renderer = createStreamingRenderer(renderFn)

    await renderer.render('A\n\nB')
    renderer.reset()
    const callsBefore = renderFn.mock.calls.length

    await renderer.render('A\n\nB')
    // reset 后缓存失效，需要重新渲染 stable 和 unstable
    expect(renderFn.mock.calls.length).toBe(callsBefore + 2)
  })
})
