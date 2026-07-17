import { describe, it, expect } from 'vitest'

interface SearchResultItem {
  title: string
  url: string
  snippet: string
}

function parseSearchResults(results: string): SearchResultItem[] {
  if (!results) return []
  try {
    if (typeof results === 'string') {
      const parsed = JSON.parse(results)
      if (Array.isArray(parsed)) return parsed
      if (parsed.results && Array.isArray(parsed.results)) return parsed.results
    }
  } catch {
    return []
  }
  return []
}

describe('parseSearchResults', () => {
  it('should parse JSON array string', () => {
    const input = JSON.stringify([{ title: 'test', url: 'http://x.com', snippet: 'snip' }])
    const result = parseSearchResults(input)
    expect(result).toHaveLength(1)
    expect(result[0].title).toBe('test')
    expect(result[0].url).toBe('http://x.com')
    expect(result[0].snippet).toBe('snip')
  })

  it('should extract inner array from nested results object', () => {
    const input = JSON.stringify({
      results: [{ title: 'test', url: 'http://x.com', snippet: 'snip' }]
    })
    const result = parseSearchResults(input)
    expect(result).toHaveLength(1)
    expect(result[0].title).toBe('test')
  })

  it('should return empty array for invalid JSON', () => {
    const result = parseSearchResults('not valid json')
    expect(result).toEqual([])
  })

  it('should return empty array for empty string', () => {
    const result = parseSearchResults('')
    expect(result).toEqual([])
  })
})
