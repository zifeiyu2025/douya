export function fixUtf8(text: string): string {
    if (!text) return ''
    return text.replace(/[\uFFFD\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, '')
}
