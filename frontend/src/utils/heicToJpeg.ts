/**
 * HEIC/HEIF 图片转 JPEG 工具
 *
 * 使用 heic-to 库的 CSP 版本（避免 unsafe-eval 被拦截），
 * 通过 Web Worker 解码 HEIC 图片为 ImageData，再用 canvas 编码为 JPEG Blob。
 *
 * 设计要点：
 * - 懒加载：仅在上传 HEIC 图片时才动态 import heic-to（~2.9MB）
 * - 模块缓存：首次加载后复用，避免重复初始化 worker
 * - CSP 兼容：使用 heic-to/csp 子路径导出，规避 unsafe-eval 限制
 * - 超时保护：60 秒超时防止恶意文件导致永久阻塞
 */

/** HEIC 转 JPEG 的质量（0~1） */
const HEIC_JPEG_QUALITY = 0.85

/** 单次 HEIC 解码超时时间（毫秒） */
const HEIC_DECODE_TIMEOUT = 60_000

interface HeicToModule {
  heicTo: (args: {
    blob: Blob
    type: string
    quality?: number
  }) => Promise<Blob>
}

let modulePromise: Promise<HeicToModule> | null = null

/**
 * 懒加载 heic-to 库的 CSP 版本并缓存模块。
 * 首次调用时动态 import，后续复用 promise。
 */
function getHeicTo(): Promise<HeicToModule> {
  if (!modulePromise) {
    // 使用 CSP 版本避免 unsafe-eval 被拦截
    // 动态 import 让 Vite 把 heic-to 拆分为独立 chunk，首屏不加载
    modulePromise = import('heic-to/csp').then((mod: unknown) => {
      const m = mod as HeicToModule
      if (typeof m.heicTo !== 'function') {
        throw new Error('heic-to 模块加载失败：heicTo 函数不存在')
      }
      return m
    })
  }
  return modulePromise
}

/**
 * 判断 MIME 类型是否为 HEIC/HEIF
 */
export function isHeicMimeType(mimeType: string): boolean {
  const normalized = mimeType.trim().toLowerCase()
  return normalized === 'image/heic' || normalized === 'image/heif'
}

/**
 * 把 HEIC/HEIF 文件转为 JPEG data URL
 *
 * @param file HEIC 图片的 File 或 Blob 对象
 * @returns JPEG 格式的 data URL 字符串
 * @throws 解码失败或超时抛出异常
 */
export async function heicFileToJpegDataURL(file: File | Blob): Promise<string> {
  const { heicTo } = await getHeicTo()

  // 用 Promise.race 添加超时保护，防止恶意文件导致永久阻塞
  const decodePromise = heicTo({
    blob: file,
    type: 'image/jpeg',
    quality: HEIC_JPEG_QUALITY,
  })

  const timeoutPromise = new Promise<never>((_, reject) => {
    setTimeout(() => reject(new Error('HEIC 解码超时（60秒）')), HEIC_DECODE_TIMEOUT)
  })

  const jpegBlob = await Promise.race([decodePromise, timeoutPromise])

  // Blob → data URL
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = () => reject(new Error('JPEG Blob 读取失败'))
    reader.readAsDataURL(jpegBlob)
  })
}
