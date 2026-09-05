/**
 * 剪贴板工具
 *
 * navigator.clipboard 仅在安全上下文（https / wails:// 等）可用，
 * 失败或不可用时降级为隐藏 textarea + execCommand('copy')，
 * 保证在 WebView 降级、非安全上下文等场景下复制仍可用。
 */

export async function copyText(text: string): Promise<boolean> {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 权限被拒或实现异常时走降级路径，不直接抛出
    }
  }
  return legacyCopy(text)
}

function legacyCopy(text: string): boolean {
  const textarea = document.createElement('textarea')
  textarea.value = text
  // 移出可视区域但保持可选中，避免页面滚动跳动
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  let ok: boolean
  try {
    ok = document.execCommand('copy')
  } catch {
    ok = false
  }
  document.body.removeChild(textarea)
  return ok
}
