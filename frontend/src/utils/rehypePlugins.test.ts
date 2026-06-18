/**
 * Markdown rehype 插件共享模块测试
 */
import { describe, it, expect } from 'vitest'
import { remark } from 'remark'
import remarkRehype from 'remark-rehype'
import rehypeStringify from 'rehype-stringify'
import { rehypeMermaidPre, rehypeExternalLinks, hastToString } from './rehypePlugins'

describe('rehypeMermaidPre', () => {
  it('将 mermaid 代码块转换为 div.mermaid', async () => {
    const md = '```mermaid\ngraph TD\nA-->B\n```'
    const result = await remark()
      .use(remarkRehype)
      .use(rehypeMermaidPre)
      .use(rehypeStringify)
      .process(md)
    const html = String(result)
    expect(html).toContain('<div class="mermaid">')
    expect(html).not.toContain('<code class="language-mermaid">')
  })

  it('保留普通代码块不变', async () => {
    const md = '```js\nconsole.log(1)\n```'
    const result = await remark()
      .use(remarkRehype)
      .use(rehypeMermaidPre)
      .use(rehypeStringify)
      .process(md)
    const html = String(result)
    expect(html).toContain('<code class="language-js">')
    expect(html).not.toContain('class="mermaid"')
  })
})

describe('rehypeExternalLinks', () => {
  it('为外部链接添加 target=_blank', async () => {
    const md = '[example](https://example.com)'
    const result = await remark()
      .use(remarkRehype)
      .use(rehypeExternalLinks)
      .use(rehypeStringify)
      .process(md)
    const html = String(result)
    expect(html).toContain('target="_blank"')
    expect(html).toContain('rel="noopener noreferrer"')
  })

  it('不为内部链接添加 target', async () => {
    const md = '[internal](/path)'
    const result = await remark()
      .use(remarkRehype)
      .use(rehypeExternalLinks)
      .use(rehypeStringify)
      .process(md)
    const html = String(result)
    expect(html).not.toContain('target="_blank"')
  })
})

describe('hastToString', () => {
  it('提取文本节点', () => {
    expect(hastToString({ type: 'text', value: 'hello' })).toBe('hello')
  })

  it('递归提取子节点', () => {
    const node = {
      type: 'element',
      children: [
        { type: 'text', value: 'a' },
        { type: 'text', value: 'b' },
      ],
    }
    expect(hastToString(node)).toBe('ab')
  })

  it('空节点返回空字符串', () => {
    expect(hastToString({ type: 'element' })).toBe('')
  })
})
