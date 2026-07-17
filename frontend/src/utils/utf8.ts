export function fixUtf8(text: string): string {
  if (!text) return ''
  // eslint-disable-next-line no-control-regex -- 故意匹配控制字符，用于清理 UTF-8 文本中的无效字符
  return text.replace(/[\uFFFD\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, '')
}
