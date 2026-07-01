/**
 * markdownStreaming 工具测试
 *
 * 验证流式 Markdown 渲染的优化逻辑：
 * - 内容拆分：按块级边界（空行）拆分为稳定块和不稳定块
 */
import { describe, it, expect } from 'vitest'
import { splitStableUnstable } from '../utils/markdownStreaming'

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

  // 行边界拆分：无空行时回退到按最后一个 \n 拆分，最后一行为 unstable
  it('无空行时按最后一个换行拆分，最后一行为不稳定块', () => {
    const content = '第一行\n第二行\n第三行'
    const { stable, unstable } = splitStableUnstable(content)
    expect(stable).toBe('第一行\n第二行\n')
    expect(unstable).toBe('第三行')
  })

  it('单行无换行时全部为不稳定块', () => {
    const content = '只有一行没有换行'
    const { stable, unstable } = splitStableUnstable(content)
    expect(stable).toBe('')
    expect(unstable).toBe('只有一行没有换行')
  })

  it('以换行结尾时最后一行为空，全部为稳定块', () => {
    const content = '第一行\n第二行\n'
    const { stable, unstable } = splitStableUnstable(content)
    expect(stable).toBe('第一行\n第二行\n')
    expect(unstable).toBe('')
  })
})
