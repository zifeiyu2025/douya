/**
 * sectionSearch —— 设置页章节搜索的纯逻辑（与视图解耦，便于单元测试）
 *
 * 对标 VS Code 设置搜索：输入即过滤，仅保留匹配章节。
 * id 同时承担两个职责：目录锚点滚动定位 + 路由 open 参数对齐（如 open=model-download）。
 * keywords 覆盖子组件内主要设置项名称，扩大命中面。
 */

export interface SettingsSection {
  no: number
  id: string
  title: string
  desc: string
  keywords: string[]
}

export const SETTINGS_SECTIONS: SettingsSection[] = [
  {
    no: 1,
    id: 'appearance',
    title: '外观',
    desc: '主题、背景、头像',
    keywords: ['主题', '深色', '浅色', '跟随系统', '背景', '头像', '透明']
  },
  {
    no: 2,
    id: 'chat',
    title: 'AI 对话',
    desc: '提示词、推理、生成参数、朗读',
    keywords: [
      '提示词',
      '系统提示',
      '温度',
      '采样',
      'top_p',
      'top_k',
      '上下文',
      '朗读',
      'TTS',
      '语音'
    ]
  },
  {
    no: 3,
    id: 'search',
    title: '联网搜索',
    desc: '搜索引擎、搜索密钥',
    keywords: ['搜索', '搜索引擎', '密钥', 'API Key', '联网', 'Bing', 'SearXNG']
  },
  {
    no: 4,
    id: 'performance',
    title: '性能',
    desc: 'GPU、后端、KV 缓存、推测解码',
    keywords: [
      'GPU',
      '显卡',
      '后端',
      'llama.cpp',
      'CUDA',
      'Vulkan',
      'Metal',
      'CPU',
      '线程',
      'KV 缓存',
      '推测解码',
      '加速'
    ]
  },
  {
    no: 5,
    id: 'api',
    title: 'API 服务',
    desc: '端点、密钥、局域网访问',
    keywords: ['API', '端口', 'Key', '局域网', '服务器', 'OpenAI 兼容']
  },
  {
    no: 6,
    id: 'advanced',
    title: '高级',
    desc: 'MCP 工具、RAG、LoRA、实验功能',
    keywords: ['MCP', '工具', 'RAG', '知识库', 'LoRA', '实验', '日志', '诊断']
  },
  {
    no: 7,
    id: 'model-download',
    title: '模型下载',
    desc: 'ModelScope / HF 镜像内置下载',
    keywords: ['模型', '下载', 'ModelScope', '魔搭', 'HF', 'HuggingFace', '镜像']
  },
  {
    no: 8,
    id: 'about',
    title: '关于',
    desc: '版本与更新',
    keywords: ['关于', '版本', '更新', '反馈', '群']
  }
]

/** 判断章节是否匹配搜索词（匹配标题/描述/关键词，不区分大小写）；空查询恒为 true */
export function sectionMatches(section: SettingsSection, query: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  const haystack = [section.title, section.desc, ...section.keywords].join(' ').toLowerCase()
  return haystack.includes(q)
}

/** 按搜索词过滤章节列表（空查询返回全部，顺序不变） */
export function filterSections(sections: SettingsSection[], query: string): SettingsSection[] {
  return sections.filter(s => sectionMatches(s, query))
}
