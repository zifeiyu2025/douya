/**
 * 图片预处理工具
 *
 * 整合自 llama.cpp 原生 webui 的附件处理优点：
 * - HEIC/HEIF → JPEG 转换（heic-to 库懒加载，CSP 兼容）
 * - SVG/WebP → PNG 格式归一化（stb_image 不支持这两种格式解码）
 * - JPEG EXIF 方向修正（stb_image 忽略 EXIF，需前端把方向烤进像素）
 * - 兆像素级尺寸限制（防止超大图撑爆 LLM 上下文）
 *
 * 设计原则：
 * - 零次重编码优化：无需处理的图片直接 pass through
 * - 合并 canvas pass：尺寸限制和 EXIF 烤入在一次 canvas 绘制中完成
 * - 有界扫描：EXIF 只读前 128KB，避免整文件 atob
 */
// F-1.16：isHeicMimeType 抽取到 heicToJpeg.ts 统一导出，消除两处重复定义
// heicFileToJpegDataURL 同为静态 import：heic-to 库的懒加载由 heicToJpeg.ts
// 内部的 getHeicTo() → import('heic-to/csp') 实现，不依赖本文件的动态 import
import { isHeicMimeType, heicFileToJpegDataURL } from './heicToJpeg'

// ===== 常量 =====

/** 默认兆像素上限：4MP（约 2680×1496，平衡清晰度与 token 消耗） */
const DEFAULT_MAX_MEGAPIXELS = 4

/** 1 兆像素 = 100 万像素 */
const MEGAPIXELS_TO_PIXELS = 1_000_000

/** EXIF 扫描字节上限：只扫前 128KB，避免整文件 atob */
const EXIF_SCAN_BYTE_LIMIT = 128 * 1024

/** JPEG 段标记常量 */
const JPEG_SOI_MARKER = 0xffd8
const APP1_MARKER = 0xe1
const SOS_MARKER = 0xda
const EXIF_SIGNATURE = 0x45786966 // "Exif"
const TIFF_LITTLE_ENDIAN = 0x4949 // "II"
const TIFF_BIG_ENDIAN = 0x4d4d // "MM"
const TIFF_MAGIC = 42
const EXIF_ORIENTATION_TAG = 0x0112
const IFD_ENTRY_SIZE = 12

/**
 * 需要归一化为 PNG 的 MIME 类型。
 * 注：BMP 实际上被 stb_image 支持，所以不在此列表中；
 * JPEG/PNG/GIF/BMP 直接 pass through，避免无谓重编码。
 */
const NORMALIZE_TO_PNG = new Set([
  'image/svg+xml',
  'image/webp',
])

// ===== EXIF 方向读取 =====

/**
 * 从 JPEG data URL 读取 EXIF orientation 值。
 * 只扫描前 128KB，避免整文件 atob 解码。
 * 非 JPEG 或无 EXIF 返回 1（默认方向）。
 */
function getJpegOrientation(dataUrl: string): number {
  try {
    const payloadStart = dataUrl.indexOf(',')
    if (payloadStart <= 0) return 1

    // 只解码有界前缀（APP1 段总在文件开头附近）
    const charLimit = Math.ceil(EXIF_SCAN_BYTE_LIMIT / 3) * 4
    const slice = dataUrl.slice(payloadStart + 1, payloadStart + 1 + charLimit)
    const binary = atob(slice.slice(0, slice.length - (slice.length % 4)))
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i)
    }

    return findExifOrientation(new DataView(bytes.buffer))
  } catch {
    return 1
  }
}

/**
 * 在 JPEG 字节流中查找 EXIF orientation。
 * 走 JPEG 段结构：SOI → 跳过非 APP1 段 → 遇到 APP1 段则解析 → 遇到 SOS 放弃。
 */
function findExifOrientation(view: DataView): number {
  if (view.byteLength < 2) return 1

  // 校验 SOI 标记（JPEG 文件以 0xFFD8 开头）
  if (view.getUint16(0) !== JPEG_SOI_MARKER) return 1

  let offset = 2
  while (offset < view.byteLength) {
    // 每个段以 0xFF 开头
    if (view.getUint8(offset) !== 0xff) break
    const marker = view.getUint16(offset)
    offset += 2

    // SOS = 压缩数据起点，后面不再是元数据
    if (marker === SOS_MARKER) break

    // 段长度（包含自身 2 字节）
    if (offset + 2 > view.byteLength) break
    const segmentLength = view.getUint16(offset)
    if (segmentLength < 2) break

    if (marker === APP1_MARKER) {
      // APP1 段：检查是否为 EXIF
      const orientation = parseExifOrientation(view, offset + 2, offset + segmentLength)
      if (orientation > 0) return orientation
    }

    offset += segmentLength
  }

  return 1
}

/**
 * 解析 APP1 段中的 EXIF orientation。
 * 校验 "Exif\0\0" 签名 + TIFF 字节序 + magic 42，再扫描 IFD0 找 orientation tag。
 */
function parseExifOrientation(view: DataView, start: number, end: number): number {
  // 校验 "Exif\0\0" 签名（6 字节）
  if (start + 6 > end) return 0
  if (view.getUint32(start) !== EXIF_SIGNATURE) return 0

  // 跳过 "Exif\0\0"，进入 TIFF 头
  const tiffStart = start + 6
  if (tiffStart + 8 > end) return 0

  // 读取 TIFF 字节序
  const byteOrder = view.getUint16(tiffStart)
  const littleEndian = byteOrder === TIFF_LITTLE_ENDIAN
  if (!littleEndian && byteOrder !== TIFF_BIG_ENDIAN) return 0

  // 校验 TIFF magic 42
  const magic = view.getUint16(tiffStart + 2, littleEndian)
  if (magic !== TIFF_MAGIC) return 0

  // IFD0 偏移（相对于 tiffStart）
  const ifdOffset = view.getUint32(tiffStart + 4, littleEndian)
  const ifdStart = tiffStart + ifdOffset
  if (ifdStart + 2 > end) return 0

  // 扫描 IFD0 表项找 orientation tag (0x0112)
  const entryCount = view.getUint16(ifdStart, littleEndian)
  for (let i = 0; i < entryCount; i++) {
    const entryOffset = ifdStart + 2 + i * IFD_ENTRY_SIZE
    if (entryOffset + IFD_ENTRY_SIZE > end) break

    const tag = view.getUint16(entryOffset, littleEndian)
    if (tag === EXIF_ORIENTATION_TAG) {
      // orientation 是 SHORT 类型（type=3），值在 entryOffset+8 处
      return view.getUint16(entryOffset + 8, littleEndian)
    }
  }

  return 0
}

// ===== 图片加载与 canvas 工具 =====

/** 加载 dataUrl 为 HTMLImageElement */
function loadImage(dataUrl: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('图片加载失败'))
    img.src = dataUrl
  })
}

/**
 * 把图片绘制到 canvas 并输出为 dataUrl。
 * 可选填白底（用于 SVG/WebP 透明通道归一化）。
 */
function drawToCanvas(
  img: HTMLImageElement,
  width: number,
  height: number,
  mimeType: string,
  backgroundColor?: string
): string {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('无法获取 2D canvas 上下文')

  // 透明格式（SVG/WebP）转 PNG 时填白底
  if (backgroundColor) {
    ctx.fillStyle = backgroundColor
    ctx.fillRect(0, 0, width, height)
  }

  ctx.drawImage(img, 0, 0, width, height)
  return canvas.toDataURL(mimeType)
}

// ===== 格式归一化 =====

/**
 * 把 SVG/WebP 归一化为 PNG。
 * 原因：llama.cpp 后端 stb_image 不支持 SVG 和 WebP 解码。
 * 其他格式（JPEG/PNG/GIF/BMP）直接 pass through，零次重编码。
 *
 * @returns 归一化后的 dataUrl 和最终 MIME 类型
 */
export async function normalizeImageFormat(
  dataUrl: string,
  mimeType: string
): Promise<{ dataUrl: string; mimeType: string }> {
  if (!NORMALIZE_TO_PNG.has(mimeType)) {
    return { dataUrl, mimeType }
  }

  const img = await loadImage(dataUrl)
  const width = img.naturalWidth || 300
  const height = img.naturalHeight || 300

  // SVG/WebP 可能带透明通道，转 PNG 时填白底
  const normalizedDataUrl = drawToCanvas(img, width, height, 'image/png', 'white')
  return { dataUrl: normalizedDataUrl, mimeType: 'image/png' }
}

// ===== 兆像素限制 + EXIF 方向烤入 =====

/**
 * 兆像素级尺寸限制 + JPEG EXIF 方向烤入（合并到一次 canvas pass）。
 *
 * 工作原理：
 * - 浏览器解码 JPEG 时已把 EXIF 方向应用到 naturalWidth/naturalHeight
 *   （即描述的是直立图像的尺寸）
 * - 但 llama.cpp 后端 stb_image 忽略 EXIF，所以需要前端把方向"烤进"像素
 * - 既不需要缩放也不需要旋转时直接 pass through，零次重编码
 *
 * @param dataUrl 图片 data URL
 * @param maxMegapixels 兆像素上限（默认 4MP），传 0 表示不限制
 * @param orientation EXIF orientation 值（1 表示默认方向，>1 表示需要旋转烤入）
 */
export async function capImageSize(
  dataUrl: string,
  maxMegapixels: number = DEFAULT_MAX_MEGAPIXELS,
  orientation: number = 1
): Promise<string> {
  const img = await loadImage(dataUrl)

  const targetWidth = img.naturalWidth
  const targetHeight = img.naturalHeight
  const totalPixels = targetWidth * targetHeight
  const maxPixels = Math.floor(maxMegapixels * MEGAPIXELS_TO_PIXELS)

  const needCap = maxPixels > 0 && totalPixels > maxPixels
  const needRotate = orientation > 1

  // 既不需要缩放也不需要旋转 → 原样返回，避免无谓重编码
  if (!needCap && !needRotate) {
    return dataUrl
  }

  let canvasWidth = targetWidth
  let canvasHeight = targetHeight

  if (needCap) {
    // 按平方根缩放保持宽高比
    const scaleFactor = Math.sqrt(maxPixels / totalPixels)
    canvasWidth = Math.floor(targetWidth * scaleFactor)
    canvasHeight = Math.floor(targetHeight * scaleFactor)
  }

  // 保留原 MIME 类型（JPEG 保持 JPEG 避免无谓转码，其他转为 PNG）
  const resultMime = dataUrl.startsWith('data:image/jpeg') ? 'image/jpeg' : 'image/png'
  return drawToCanvas(img, canvasWidth, canvasHeight, resultMime)
}

// ===== 文件读取工具 =====

/**
 * 读取 File 为 data URL（F-1.17：导出供 SettingsView.vue 复用）
 * 生活类比：像把纸质文件扫描成电子版——FileReader 是扫描仪，
 * readAsDataURL 是扫描操作，结果是带 MIME 前缀的 base64 字符串。
 */
export function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = () => reject(new Error('文件读取失败'))
    reader.readAsDataURL(file)
  })
}

// ===== 完整流水线 =====

/**
 * 图片预处理完整流水线。
 * 依次执行：[HEIC→JPEG] → 格式归一化 → EXIF 方向读取 → 兆像素限制+方向烤入
 *
 * HEIC 特殊处理：浏览器无法用 <img> 解码 HEIC dataURL，必须先用 heic-to 转为 JPEG Blob，
 * 再走 JPEG 流水线。heic-to 库懒加载（~2.9MB），仅上传 HEIC 时加载。
 *
 * @param file 用户上传的 File 对象
 * @returns 处理后的 dataUrl 和最终 MIME 类型
 */
export async function processImagePipeline(
  file: File
): Promise<{ dataUrl: string; mimeType: string }> {
  // 0. HEIC/HEIF 预处理：先转为 JPEG data URL，再走标准流水线
  //    必须在 readFileAsDataURL 之前，因为浏览器无法解码 HEIC dataURL
  let dataUrl: string
  let effectiveMime: string
  if (isHeicMimeType(file.type)) {
    // heic-to 库的懒加载由 heicToJpeg.ts 内部的 getHeicTo() 实现
    dataUrl = await heicFileToJpegDataURL(file)
    effectiveMime = 'image/jpeg'
  } else {
    // 1. 读取文件为 data URL
    dataUrl = await readFileAsDataURL(file)
    effectiveMime = file.type
  }

  // 2. 格式归一化（SVG/WebP → PNG，其他 pass through）
  const { dataUrl: normalizedUrl, mimeType: normalizedMime } = await normalizeImageFormat(
    dataUrl,
    effectiveMime
  )

  // 3. 读取 EXIF orientation（仅 JPEG）
  const orientation = normalizedMime === 'image/jpeg' ? getJpegOrientation(normalizedUrl) : 1

  // 4. 兆像素限制 + EXIF 方向烤入（一次 canvas pass）
  const finalUrl = await capImageSize(normalizedUrl, DEFAULT_MAX_MEGAPIXELS, orientation)

  return { dataUrl: finalUrl, mimeType: normalizedMime }
}
