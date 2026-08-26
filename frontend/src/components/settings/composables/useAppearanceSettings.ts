import { computed } from 'vue'
import type { MessageApi, UploadCustomRequestOptions } from 'naive-ui'
import { wails } from '../../../services/wails'
// readFileAsDataURL 抽取到 imageProcess.ts 统一导出，消除两处重复定义
import { readFileAsDataURL } from '../../../utils/imageProcess'
import defaultUserAvatar from '../../../assets/images/user-avatar.svg'
import defaultAiAvatar from '../../../assets/images/appicon.png'
import type { SettingsCore } from './useSettingsCore'

/**
 * 外观域：聊天背景图、用户/AI 头像。
 * 自 SettingsView 迁出，方法体逐字保留。
 */
export function useAppearanceSettings(core: SettingsCore, message: MessageApi) {
  const { formConfig } = core

  const backgroundImageUrl = computed(() => {
    const bg = formConfig.value.chat_background
    if (!bg) return ''
    if (bg.startsWith('data:')) return bg
    return '/local-file/' + encodeURIComponent(bg)
  })

  async function selectBackgroundImage() {
    try {
      const filePath = await wails.selectImageFile()
      if (filePath) {
        formConfig.value.chat_background = filePath
      }
    } catch {
      message.destroyAll()
      message.error('选择图片失败')
    }
  }

  function clearBackground() {
    formConfig.value.chat_background = ''
    formConfig.value.chat_background_opacity = 0.85
  }

  // maxAvatarSize 头像文件最大大小（1MB）
  const maxAvatarSize = 1024 * 1024

  /**
   * 处理头像上传（用户头像和 AI 头像共用）。
   * 上传与压缩流程对两种头像完全一致，仅写入的字段不同。
   * @param data n-upload custom-request 回调传入的数据
   * @param fieldName 要写入 formConfig 的字段名（'user_avatar' 或 'ai_avatar'）
   */
  async function handleAvatarUpload(
    data: UploadCustomRequestOptions,
    fieldName: 'user_avatar' | 'ai_avatar'
  ) {
    const file = data.file.file as File
    if (file.size > maxAvatarSize) {
      message.destroyAll()
      message.error('头像图片大小不能超过 1MB')
      return
    }
    try {
      const base64 = await readFileAsDataURL(file)
      formConfig.value[fieldName] = base64
    } catch {
      message.destroyAll()
      message.error('上传失败')
    }
  }

  function clearUserAvatar() {
    formConfig.value.user_avatar = ''
  }

  function clearAIAvatar() {
    formConfig.value.ai_avatar = ''
  }

  return {
    backgroundImageUrl,
    selectBackgroundImage,
    clearBackground,
    handleAvatarUpload,
    clearUserAvatar,
    clearAIAvatar,
    defaultUserAvatar,
    defaultAiAvatar
  }
}
