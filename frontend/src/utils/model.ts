// 量化后缀正则：匹配模型名末尾的量化后缀（如 -Q4_K_M、-IQ4_XS、-BF16）。
// 注意：此正则与 Go 端 internal/llm/preset.go 的 quantSuffixRe 必须保持完全一致，
// 修改时必须两端同步，否则前后端对模型量化后缀的识别结果会不一致。
const QUANT_SUFFIX_RE = /-(Q\d+(_[A-Z0-9]+)+|IQ\d+_[A-Z0-9]+|BF16|F16|F32)$/i

const QUANT_SUFFIX_IN_FILENAME_RE = /-(Q\d+(_[A-Z0-9]+)+|IQ\d+_[A-Z0-9]+|BF16|F16|F32)(?=\.gguf$)/i

const MAX_DISPLAY_LENGTH = 20

function stripQuantSuffix(name: string): string {
  return name.replace(QUANT_SUFFIX_RE, '')
}

export function extractQuantSuffix(name: string): string {
  const match = name.match(QUANT_SUFFIX_IN_FILENAME_RE)
  return match ? '-' + match[1] : ''
}

function truncateModelName(name: string): string {
  if (name.length <= MAX_DISPLAY_LENGTH) return name
  const parts = name.split('-')
  if (parts.length > 2) {
    return parts.slice(0, 2).join('-') + '…'
  } else if (parts.length === 2) {
    return parts[0] + '…'
  }
  return name.slice(0, MAX_DISPLAY_LENGTH) + '…'
}

export function formatModelName(name: string): { display: string; full: string } {
  const full = name
  const display = truncateModelName(name)
  return { display, full }
}

export function formatModelNameFromPath(path: string): { display: string; full: string } {
  const fileName = path.split(/[/\\]/).pop() || ''
  const raw = fileName.replace(/\.gguf$/i, '')
  const display = truncateModelName(stripQuantSuffix(raw))
  return { display, full: raw }
}

/** 字节数格式化为人类可读大小（未知/非法返回"未知大小"） */
export function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '未知大小'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`
}

/** 下载进度状态 → 中文文案 */
export function downloadStatusText(status: string): string {
  switch (status) {
    case 'downloading':
      return '下载中'
    case 'completed':
      return '已完成'
    case 'failed':
      return '失败'
    case 'paused':
      return '已暂停（保留断点）'
    case 'waiting':
      return '等待中'
    default:
      return status
  }
}

// 常见嵌入模型名片段（小写匹配）。
// 用于兜底识别"只能做向量化/检索、不能聊天"的嵌入模型，
// 即使后端能力检测（text_generation）暂时拿不到，也能拦住常见误选。
const EMBEDDING_NAME_MARKERS = [
  'bge',
  'text-embedding',
  'text_embedding',
  'embedding',
  'embeddings',
  'gte-',
  'm3e',
  '-e5-',
  'e5-',
  'jina-embeddings',
  'nomic-embed',
  'mxbai-embed',
  'qwen3-embedding',
  'bce-embedding',
  'acge',
  'stella',
  'sentence-',
  'conan-embed'
]

/**
 * 判断模型名是否属于嵌入模型（不能聊天，只能做向量化/检索）。
 * 使用小写匹配，命中常见嵌入模型名称片段即视为嵌入模型。
 */
export function isEmbeddingModelName(name: string): boolean {
  if (!name) return false
  const lower = name.toLowerCase()
  return EMBEDDING_NAME_MARKERS.some(marker => lower.includes(marker))
}
