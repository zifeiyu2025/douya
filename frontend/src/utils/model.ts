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
