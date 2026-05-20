import { describe, it, expect, vi } from 'vitest'
import { renderMarkdown, renderMarkdownInline } from '../utils/markdown'

vi.mock('mermaid', () => ({
    default: {
        initialize: vi.fn(),
        render: vi.fn().mockResolvedValue({ svg: '<svg></svg>' }),
    },
}))

describe('renderMarkdown', () => {
    it('should render headings', () => {
        const result = renderMarkdown('# Hello')
        expect(result).toContain('<h1>')
        expect(result).toContain('Hello')
        expect(result).toContain('</h1>')
    })

    it('should render bold text', () => {
        const result = renderMarkdown('**bold**')
        expect(result).toContain('<strong>bold</strong>')
    })

    it('should render italic text', () => {
        const result = renderMarkdown('*italic*')
        expect(result).toContain('<em>italic</em>')
    })

    it('should render links', () => {
        const result = renderMarkdown('[text](https://example.com)')
        expect(result).toContain('<a href="https://example.com">text</a>')
    })

    it('should highlight code blocks with hljs', () => {
        const result = renderMarkdown('```go\nfmt.Println("hello")\n```')
        expect(result).toContain('class="hljs"')
        expect(result).toContain('<pre')
        expect(result).toContain('<code>')
    })

    it('should render mermaid code blocks as div with mermaid class', () => {
        const result = renderMarkdown('```mermaid\ngraph TD\n```')
        expect(result).toContain('class="mermaid"')
        expect(result).toContain('<div')
    })

    it('should escape XSS script tags', () => {
        const result = renderMarkdown("<script>alert('xss')</script>")
        expect(result).not.toContain('<script>')
        expect(result).toContain('&lt;script&gt;')
    })
})

describe('renderMarkdownInline', () => {
    it('should not wrap inline content in p tags', () => {
        const result = renderMarkdownInline('**bold**')
        expect(result).toContain('<strong>bold</strong>')
        expect(result).not.toContain('<p>')
    })
})
