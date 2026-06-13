import { describe, it, expect, vi } from 'vitest'

// Mock mermaid
vi.mock('mermaid', () => ({
    default: {
        initialize: vi.fn(),
        render: vi.fn().mockResolvedValue({ svg: '<svg></svg>' }),
    },
}))

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

    it('should render mermaid code blocks as div with mermaid class', async () => {
        const result = await renderMarkdown('```mermaid\ngraph TD\n```')
        expect(result).toContain('mermaid')
    })

    it('should escape XSS script tags', async () => {
        const result = await renderMarkdown("<script>alert('xss')</script>")
        expect(result).not.toContain('<script>')
    })

    it('should render inline math with KaTeX', async () => {
        const result = await renderMarkdown('$x^2 + y^2 = z^2$')
        expect(result).toContain('katex')
    })

    it('should render display math with KaTeX', async () => {
        const result = await renderMarkdown('$$\nE = mc^2\n$$')
        expect(result).toContain('katex')
    })

    it('should not treat currency as math', async () => {
        const result = await renderMarkdown('Price: $5.99')
        expect(result).not.toContain('katex')
    })

    it('should preserve backslash in LaTeX commands', async () => {
        const result = await renderMarkdown('$\\times$')
        expect(result).toContain('katex')
    })
})
