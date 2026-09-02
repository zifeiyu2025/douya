/**
 * 附件上传共享常量与能力校验。
 * ChatInput（粘贴/拖拽路径）与 ChatToolbar（附件菜单路径）共用一份，
 * 修改可接受的扩展名或提示文案只需改这里。
 */
import type { ModelCapabilities } from '../types/chat'

export const IMAGE_ACCEPT = '.jpg,.jpeg,.png,.gif,.webp,.bmp,.svg,.heic,.heif'
export const AUDIO_ACCEPT = '.wav,.mp3,.ogg,.flac,.aac,.m4a,.wma'
export const TEXT_ACCEPT =
  '.txt,.md,.csv,.json,.xml,.html,.htm,.css,.js,.jsx,.ts,.tsx,.vue,.svelte,.py,.go,.java,.c,.cpp,.h,.hpp,.rs,.sh,.bat,.yaml,.yml,.toml,.ini,.cfg,.log,.sql,.adoc,.tex,.bib,.cs,.kt,.swift,.dart,.r,.scala,.hs,.cu,.cuh,.comp,.properties'
export const PDF_ACCEPT = '.pdf'
export const DOCX_ACCEPT = '.docx'
export const VIDEO_ACCEPT = '.mp4,.webm,.avi,.mov,.mkv,.wmv,.flv'

/** 按附件类型返回文件选择器的 accept 过滤串 */
export function getAcceptForType(type: string): string {
  switch (type) {
    case 'image':
      return IMAGE_ACCEPT
    case 'audio':
      return AUDIO_ACCEPT
    case 'text':
      return TEXT_ACCEPT
    case 'pdf':
      return PDF_ACCEPT
    case 'docx':
      return DOCX_ACCEPT
    case 'video':
      return VIDEO_ACCEPT
    default:
      return ''
  }
}

/**
 * 校验当前模型能力是否支持该附件类型。
 * 不支持时返回给用户的提示文案；支持返回 null。
 */
export function checkUploadCapability(
  type: string,
  caps: Pick<
    ModelCapabilities,
    'mmproj_loaded' | 'image_input' | 'audio_input' | 'video_input' | 'text_input'
  >
): string | null {
  if ((type === 'image' || type === 'audio' || type === 'video') && !caps.mmproj_loaded) {
    return '多模态投影未加载，无法处理此类型文件'
  }
  if (type === 'image' && !caps.image_input) {
    return '当前模型不支持图片输入'
  }
  if (type === 'audio' && !caps.audio_input) {
    return '当前模型不支持音频输入'
  }
  if (type === 'video' && !caps.video_input) {
    return '当前模型不支持视频输入'
  }
  if ((type === 'text' || type === 'pdf' || type === 'docx') && !caps.text_input) {
    return '当前模型不支持文本文件输入'
  }
  return null
}
