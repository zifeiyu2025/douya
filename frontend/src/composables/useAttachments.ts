// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

import type { ShallowRef } from 'vue'
import { processImagePipeline } from '../utils/imageProcess'
import type { Attachment } from '../services/wails'

// 文件大小限制（单位：MB）
// 安全实践：上传文件大小限制，配合后端 200MB 总限制，防止超大文件导致 OOM 或 IPC 超时
const MAX_IMAGE_SIZE = 20
const MAX_AUDIO_SIZE = 50
const MAX_VIDEO_SIZE = 100
const MAX_PDF_SIZE = 50
const MAX_DOCX_SIZE = 50
const MAX_TEXT_SIZE = 10

// 文件类型配置：定义每种附件类型的处理规则
// 抽取原因（基于 F-1.8+F-3.2）：ChatInput.vue 中 6 个 process*File 函数结构高度相似，
// 通过配置表 + processFileCommon 高阶函数统一处理，新增类型只需添加配置项。
//
// 生活类比：像快递分拣中心的"包裹规则表"——每个包裹类型都有自己的大小限制、包装方式、
// 特殊处理（如冷冻品需温度检查），分拣员只需查表执行，无需记住每种类型的特殊规则。
interface FileProcessConfig {
  maxSize: number
  label: string
  // 计算 mime_type：从文件本身派生（部分类型固定，部分依赖 file.type）
  mimeType: (file: File) => string
  // 额外字段：如 audio 的 format 字段
  extra?: (file: File) => Record<string, string>
  // 读取模式：dataURL（base64）或 text（原文）
  readMode: 'dataURL' | 'text'
  // 后处理（仅 text 模式）：返回 null 表示中止添加（如检测到二进制内容）
  postProcess?: (result: string, file: File) => string | null
}

const FILE_CONFIGS: Record<string, FileProcessConfig> = {
  audio: {
    maxSize: MAX_AUDIO_SIZE,
    label: '音频',
    mimeType: (file) => file.type || `audio/${file.name.split('.').pop()?.toLowerCase() || 'wav'}`,
    extra: (file) => ({ format: file.name.split('.').pop()?.toLowerCase() || 'wav' }),
    readMode: 'dataURL',
  },
  pdf: {
    maxSize: MAX_PDF_SIZE,
    label: 'PDF',
    mimeType: () => 'application/pdf',
    readMode: 'dataURL',
  },
  docx: {
    maxSize: MAX_DOCX_SIZE,
    label: 'DOCX',
    mimeType: () => 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    readMode: 'dataURL',
  },
  video: {
    maxSize: MAX_VIDEO_SIZE,
    label: '视频',
    mimeType: (file) => file.type || 'video/mp4',
    readMode: 'dataURL',
  },
  text: {
    maxSize: MAX_TEXT_SIZE,
    label: '文本',
    mimeType: (file) => file.type || 'text/plain',
    readMode: 'text',
    postProcess: (result, file) => {
      // 安全实践：检测文本内容是否含大量不可打印字符（可能是伪造的文本文件），见安全审查 #37
      if (isLikelyBinaryContent(result)) {
        return null // 中止添加，调用方通过返回值判断
      }
      return result
    },
  },
}

// MessageApi 类型从 naive-ui 导入，避免在此处硬依赖 message 实例
type MessageApi = {
  error: (content: string) => void
  warning: (content: string) => void
  success: (content: string) => void
}

// isLikelyBinaryContent 检测文本内容是否含大量不可打印字符（可能是伪造的文本文件）。
// 安全实践：见安全审查 #37，防止用户将二进制文件改名为 .txt 上传。
function isLikelyBinaryContent(text: string): boolean {
  if (text.length === 0) return false
  let nonPrintable = 0
  const sample = text.slice(0, 1000) // 仅检查前 1000 字符
  for (const ch of sample) {
    const code = ch.charCodeAt(0)
    // 允许换行、回车、制表符
    if (code !== 10 && code !== 13 && code !== 9 && code < 32) {
      nonPrintable++
    }
  }
  return nonPrintable / sample.length > 0.1 // 不可打印字符超过 10% 判定为二进制
}

/**
 * useAttachments 提供附件文件的统一处理能力。
 *
 * 抽取原因（基于 F-1.8+F-3.2）：ChatInput.vue 中 6 个 process*File 函数结构高度相似，
 * 提取为 composable 后：
 *   1. 通过 FILE_CONFIGS 表驱动 + processFileCommon 高阶函数消除 5 处重复（audio/pdf/docx/video/text）
 *   2. processImageFile 因异步流水线差异保留独立实现
 *   3. ChatInput.vue 减少约 130 行代码，附件处理逻辑集中维护
 *
 * 生活类比：像一个"文件处理车间"——车间外部（组件）只管把文件丢进来和拿走结果，
 * 车间内部按照"分拣规则表"（FILE_CONFIGS）自动选择对应的处理流水线。
 *
 * @param attachments 附件数组的 shallowRef（外部持有，确保响应式由调用方控制）
 * @param message naive-ui 的 message API（用于错误/警告提示）
 */
export function useAttachments(
  attachments: ShallowRef<Attachment[]>,
  message: MessageApi
) {
  // checkFileSize 校验文件大小是否超过限制。
  // 生活类比：像快递员称重——超重的包裹直接拒收并告知原因。
  function checkFileSize(file: File, maxSizeMB: number, label: string): boolean {
    const sizeMB = file.size / (1024 * 1024)
    if (sizeMB > maxSizeMB) {
      message.error(`${label}文件大小不能超过 ${maxSizeMB}MB（当前 ${sizeMB.toFixed(1)}MB）`)
      return false
    }
    return true
  }

  // readFileWithErrorHandling 封装 FileReader 的读取逻辑，统一处理 error/abort 事件。
  function readFileWithErrorHandling(
    file: File,
    readFn: (reader: FileReader) => void,
    onSuccess: (result: string) => void,
    label: string
  ) {
    const reader = new FileReader()
    reader.onload = () => {
      onSuccess(reader.result as string)
    }
    reader.onerror = () => {
      message.error(`${label}文件读取失败，请重试`)
    }
    reader.onabort = () => {
      message.warning(`${label}文件读取已取消`)
    }
    readFn(reader)
  }

  // processFileCommon 通用文件处理高阶函数。
  // 根据配置表的 readMode 选择读取方式，postProcess 进行后处理，最后构造 attachment 追加到数组。
  //
  // 生活类比：像流水线工人按"作业指导书"（config）操作——
  // 先称重（checkFileSize），再按指定方式拆包（readFn），可能还要质检（postProcess），
  // 最后贴标签入库（构造 attachment 并 append）。
  function processFileCommon(file: File, type: string) {
    const cfg = FILE_CONFIGS[type]
    if (!cfg || !checkFileSize(file, cfg.maxSize, cfg.label)) return

    readFileWithErrorHandling(
      file,
      (reader) => {
        if (cfg.readMode === 'dataURL') {
          reader.readAsDataURL(file)
        } else {
          reader.readAsText(file)
        }
      },
      (result) => {
        let data: string
        if (cfg.readMode === 'dataURL') {
          // dataURL 格式：data:<mime>;base64,<payload>，提取 base64 部分
          data = result.split(',')[1]
        } else {
          // text 模式：可选后处理（如二进制检测），返回 null 表示中止
          const processed = cfg.postProcess?.(result, file) ?? result
          if (processed === null) {
            // 对于 text 类型的二进制检测失败，postProcess 无法直接访问 message，
            // 这里通过 isLikelyBinaryContent 重复检测并提示
            if (isLikelyBinaryContent(result)) {
              message.warning(`文件 ${file.name} 内容似乎不是文本，可能为二进制文件`)
            }
            return
          }
          data = processed
        }

        // shallowRef 需替换整个数组才能触发响应式（任务 23）
        const attachment: Attachment = {
          type,
          name: file.name,
          mime_type: cfg.mimeType(file),
          data,
          ...cfg.extra?.(file),
        }
        attachments.value = [...attachments.value, attachment]
      },
      cfg.label
    )
  }

  // processImageFile 图片处理：异步流水线（格式归一化 + EXIF 修正 + 兆像素限制）。
  // 保留独立实现的原因：图片处理是异步的（processImagePipeline 返回 Promise），
  // 且有 4 张数量限制和特殊的错误处理流程，与 processFileCommon 的同步模式不兼容。
  async function processImageFile(file: File) {
    if (!file.type.startsWith('image/')) {
      message.error('请选择图片文件')
      return
    }
    if (attachments.value.filter(a => a.type === 'image').length >= 4) {
      message.warning('最多上传 4 张图片')
      return
    }
    if (!checkFileSize(file, MAX_IMAGE_SIZE, '图片')) return

    try {
      // 图片预处理流水线：格式归一化（SVG/WebP→PNG）+ EXIF 方向修正 + 兆像素限制
      const { dataUrl, mimeType } = await processImagePipeline(file)
      // shallowRef 需替换整个数组才能触发响应式（任务 23）
      attachments.value = [...attachments.value, {
        type: 'image',
        name: file.name,
        mime_type: mimeType,
        data: dataUrl,
      }]
    } catch (err) {
      console.error('图片预处理失败:', err)
      message.error('图片处理失败，请重试或更换图片')
    }
  }

  // processFileByType 按类型分发到对应的处理函数。
  // 调用方只需提供类型和文件，无需关心具体处理逻辑。
  function processFileByType(type: string, file: File) {
    if (type === 'image') {
      processImageFile(file)
    } else {
      processFileCommon(file, type)
    }
  }

  // removeAttachment 从附件列表中移除指定索引的附件。
  function removeAttachment(idx: number) {
    // shallowRef 需替换整个数组才能触发响应式（任务 23）
    attachments.value = attachments.value.filter((_, i) => i !== idx)
  }

  return {
    processImageFile,
    processFileCommon,
    processFileByType,
    removeAttachment,
    checkFileSize,
  }
}
