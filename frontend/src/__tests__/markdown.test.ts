import { describe, it, expect, vi } from 'vitest'

// Mock DOMPurify：测试环境中没有 window，需要 mock
vi.mock('dompurify', () => {
    const sanitize = (html: string) => html.replace(/<script[\s\S]*?<\/script>/gi, '')
    return { default: { sanitize } }
})

// 动态导入，确保 mock 生效
const { renderMarkdown } = await import('../utils/markdown')

describe('renderMarkdown', () => {
    it('should render headings', async () => {
        const result = await renderMarkdown('# Hello')
        expect(result).toContain('<h1')
        expect(result).toContain('Hello')
        expect(result).toContain('</h1>')
    })

    it('should render bold text', async () => {
        const result = await renderMarkdown('**bold**')
        expect(result).toContain('<strong>bold</strong>')
    })

    it('should render italic text', async () => {
        const result = await renderMarkdown('*italic*')
        expect(result).toContain('<em>italic</em>')
    })

    it('should render links', async () => {
        const result = await renderMarkdown('[text](https://example.com)')
        expect(result).toContain('href="https://example.com"')
        expect(result).toContain('text')
    })

    it('should highlight code blocks with hljs', async () => {
        const result = await renderMarkdown('```go\nfmt.Println("hello")\n```')
        expect(result).toContain('hljs')
        expect(result).toContain('<pre')
        expect(result).toContain('<code')
    })

    it('should add code header with language label and copy button', async () => {
        const result = await renderMarkdown('```javascript\nconsole.log("hi")\n```')
        expect(result).toContain('code-header')
        expect(result).toContain('code-lang')
        expect(result).toContain('code-copy-btn')
        expect(result).toContain('javascript')
    })

    it('should add target=_blank to links', async () => {
        const result = await renderMarkdown('[text](https://example.com)')
        expect(result).toContain('target="_blank"')
        expect(result).toContain('rel="noopener noreferrer"')
    })

    it('should escape XSS script tags', async () => {
        const result = await renderMarkdown("<script>alert('xss')</script>")
        expect(result).not.toContain('<script>')
    })

    it('should render unordered lists', async () => {
        const result = await renderMarkdown('- item 1\n- item 2')
        expect(result).toContain('<ul>')
        expect(result).toContain('<li>item 1</li>')
        expect(result).toContain('<li>item 2</li>')
    })

    it('should render ordered lists', async () => {
        const result = await renderMarkdown('1. first\n2. second')
        expect(result).toContain('<ol>')
        expect(result).toContain('<li>first</li>')
        expect(result).toContain('<li>second</li>')
    })

    it('should render blockquotes', async () => {
        const result = await renderMarkdown('> quote text')
        expect(result).toContain('<blockquote>')
        expect(result).toContain('quote text')
    })

    it('should render tables (GFM)', async () => {
        const result = await renderMarkdown('| A | B |\n|---|---|\n| 1 | 2 |')
        expect(result).toContain('<table>')
    })

    it('should render horizontal rule', async () => {
        const result = await renderMarkdown('---')
        expect(result).toContain('<hr')
    })

    it('should handle empty content', async () => {
        const result = await renderMarkdown('')
        expect(result).toBe('')
    })
})
