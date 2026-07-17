import { describe, it, expect } from 'vitest'
import { chat } from '../../wailsjs/go/models'

describe('chat.Conversation', () => {
  it('should parse time fields as strings', () => {
    const source = {
      id: 'conv-1',
      title: '测试会话',
      created_at: '2026-05-06T14:30:00+08:00',
      updated_at: '2026-05-06T14:30:00+08:00'
    }

    const conv = chat.Conversation.createFrom(source)

    expect(conv.id).toBe('conv-1')
    expect(conv.title).toBe('测试会话')
    expect(typeof conv.created_at).toBe('string')
    expect(typeof conv.updated_at).toBe('string')
    expect(conv.created_at).toBe('2026-05-06T14:30:00+08:00')
    expect(conv.updated_at).toBe('2026-05-06T14:30:00+08:00')
  })

  it('should handle Chinese title without garbled text', () => {
    const source = {
      id: 'conv-2',
      title: '中文标题测试 🎊',
      created_at: '2026-05-06T14:30:00+08:00',
      updated_at: '2026-05-06T14:30:00+08:00'
    }

    const conv = chat.Conversation.createFrom(source)

    expect(conv.title).toBe('中文标题测试 🎊')
  })

  it('should not produce time.Time internal fields', () => {
    const source = {
      id: 'conv-3',
      title: 'test',
      created_at: '2026-05-06T14:30:00+08:00',
      updated_at: '2026-05-06T14:30:00+08:00'
    }

    const conv = chat.Conversation.createFrom(source)
    const created = conv.created_at as any

    expect(typeof created).toBe('string')
    expect(created).not.toHaveProperty('Wall')
    expect(created).not.toHaveProperty('Ext')
    expect(created).not.toHaveProperty('Loc')
  })

  it('should parse from JSON string', () => {
    const jsonStr = JSON.stringify({
      id: 'conv-4',
      title: 'JSON测试',
      created_at: '2026-05-06T14:30:00+08:00',
      updated_at: '2026-05-06T14:30:00+08:00'
    })

    const conv = chat.Conversation.createFrom(jsonStr)

    expect(conv.title).toBe('JSON测试')
    expect(typeof conv.created_at).toBe('string')
  })
})

describe('chat.Message', () => {
  it('should parse time field as string', () => {
    const source = {
      id: 'msg-1',
      conversation_id: 'conv-1',
      role: 'assistant',
      content: '你好世界',
      thinking_content: '思考中',
      search_results: '[{"title":"test"}]',
      created_at: '2026-05-06T14:30:00+08:00'
    }

    const msg = chat.Message.createFrom(source)

    expect(msg.id).toBe('msg-1')
    expect(msg.content).toBe('你好世界')
    expect(msg.thinking_content).toBe('思考中')
    expect(typeof msg.created_at).toBe('string')
    expect(msg.created_at).toBe('2026-05-06T14:30:00+08:00')
  })

  it('should handle empty optional fields', () => {
    const source = {
      id: 'msg-2',
      conversation_id: 'conv-1',
      role: 'user',
      content: 'hello',
      created_at: '2026-05-06T14:30:00+08:00'
    }

    const msg = chat.Message.createFrom(source)

    expect(msg.thinking_content).toBeUndefined()
    expect(msg.search_results).toBeUndefined()
  })

  it('should handle Chinese content without garbled text', () => {
    const chineseContent = '这是一段中文内容，包含特殊字符：<>&"\' 以及 emoji 🚀'
    const source = {
      id: 'msg-3',
      conversation_id: 'conv-1',
      role: 'user',
      content: chineseContent,
      created_at: '2026-05-06T14:30:00+08:00'
    }

    const msg = chat.Message.createFrom(source)

    expect(msg.content).toBe(chineseContent)
  })
})
