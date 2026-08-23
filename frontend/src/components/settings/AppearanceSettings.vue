<!--
  AppearanceSettings: 外观设置
  纯视觉设置：主题模式、聊天背景、头像。
  背景预览以模拟聊天气泡的方式展示效果。
-->
<template>
  <!-- 主题模式 -->
  <n-form-item label="主题模式">
    <n-radio-group :value="themeStore.mode" @update:value="handleModeChange">
      <n-radio value="light">亮色</n-radio>
      <n-radio value="dark">深色</n-radio>
      <n-radio value="auto">跟随系统</n-radio>
    </n-radio-group>
  </n-form-item>

  <!-- 聊天背景 -->
  <n-form-item label="聊天背景">
    <div class="bg-preview-wrapper">
      <!-- 无背景：上传占位 -->
      <div
        v-if="!formConfig.chat_background"
        class="bg-upload-placeholder"
        @click="selectBackgroundImage"
      >
        <div class="bg-upload-icon">🖼️</div>
        <span class="bg-upload-text">点击选择背景图片</span>
        <span class="bg-upload-hint">支持 JPG、PNG、WebP 格式</span>
      </div>
      <!-- 有背景：聊天气泡预览 -->
      <div v-else class="bg-preview-area" @click="selectBackgroundImage">
        <div class="bg-preview-image" :style="{ backgroundImage: `url(${backgroundImageUrl})` }">
          <div class="bg-preview-chat">
            <div class="bg-preview-bubble bg-preview-bubble--user">
              你好，帮我总结一下今天的新闻
            </div>
            <div class="bg-preview-bubble bg-preview-bubble--ai">好的，今天的主要新闻包括...</div>
          </div>
          <div class="bg-hover-overlay">
            <span class="bg-hover-text">点击更换背景</span>
          </div>
        </div>
        <div class="bg-preview-actions">
          <span class="bg-preview-label">当前背景</span>
          <n-button size="small" type="primary" ghost @click.stop="clearBackground">
            清除背景
          </n-button>
        </div>
      </div>
    </div>
  </n-form-item>

  <!-- 背景透明度 -->
  <n-form-item v-if="formConfig.chat_background" label="背景透明度">
    <n-slider v-model:value="formConfig.chat_background_opacity" :min="0.2" :max="1" :step="0.05" />
    <span class="slider-value">{{ Math.round(formConfig.chat_background_opacity * 100) }}%</span>
  </n-form-item>

  <!-- 用户头像 -->
  <n-form-item label="用户头像">
    <div class="avatar-row">
      <div class="avatar-preview avatar-round">
        <img :src="formConfig.user_avatar || defaultUserAvatar" class="avatar-img" />
      </div>
      <div class="avatar-btns">
        <n-upload
          :show-file-list="false"
          :custom-request="data => handleAvatarUpload(data, 'user_avatar')"
          accept="image/*"
        >
          <n-button size="small" ghost>上传</n-button>
        </n-upload>
        <n-button v-if="formConfig.user_avatar" size="small" ghost @click="clearUserAvatar">
          清除
        </n-button>
      </div>
    </div>
  </n-form-item>

  <!-- AI 头像 -->
  <n-form-item label="AI 头像">
    <div class="avatar-row">
      <div class="avatar-preview avatar-round">
        <img :src="formConfig.ai_avatar || defaultAiAvatar" class="avatar-img" />
      </div>
      <div class="avatar-btns">
        <n-upload
          :show-file-list="false"
          :custom-request="data => handleAvatarUpload(data, 'ai_avatar')"
          accept="image/*"
        >
          <n-button size="small" ghost>上传</n-button>
        </n-upload>
        <n-button v-if="formConfig.ai_avatar" size="small" ghost @click="clearAIAvatar">
          清除
        </n-button>
      </div>
    </div>
  </n-form-item>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { NFormItem, NRadioGroup, NRadio, NSlider, NButton, NUpload } from 'naive-ui'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import { useThemeStore } from '../../stores/theme'

defineOptions({ name: 'AppearanceSettings' })

const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)
if (!ctx) {
  throw new Error('AppearanceSettings 必须在 SettingsView 内使用（缺少 settingsContext provide）')
}
// C-5 域切片：core 提供表单与保存，appearance 提供背景图 / 头像逻辑
const { core, appearance } = ctx
const { formConfig, autoSave } = core
const {
  backgroundImageUrl,
  selectBackgroundImage,
  clearBackground,
  handleAvatarUpload,
  clearUserAvatar,
  clearAIAvatar,
  defaultUserAvatar,
  defaultAiAvatar
} = appearance
const themeStore = useThemeStore()

function handleModeChange(mode: 'auto' | 'light' | 'dark') {
  themeStore.setMode(mode)
  autoSave()
}
</script>

<style scoped>
/* ===== 背景预览 ===== */
.bg-preview-wrapper {
  width: 100%;
}

.bg-upload-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 40px 16px;
  border: 2px dashed var(--border-color);
  border-radius: 12px;
  background: var(--bg-secondary);
  cursor: pointer;
  transition: all 0.25s;
}
.bg-upload-placeholder:hover {
  border-color: var(--accent-primary);
  background: var(--bg-hover);
}
.bg-upload-icon {
  font-size: 36px;
}
.bg-upload-text {
  font-size: 14px;
  color: var(--text-secondary);
  font-weight: 500;
}
.bg-upload-hint {
  font-size: 12px;
  color: var(--text-muted);
}

/* 有背景时的聊天气泡预览 */
.bg-preview-area {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.bg-preview-image {
  position: relative;
  height: 180px;
  border-radius: 12px;
  background-size: cover;
  background-position: center;
  overflow: hidden;
  cursor: pointer;
  border: 1px solid var(--border-color);
}
.bg-preview-chat {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 10px;
  padding: 20px 24px;
  /* VSCode 风格：暗角遮罩 + 暗化渐变，消除白雾感 */
  background: linear-gradient(
    180deg,
    rgba(0, 0, 0, 0.18) 0%,
    rgba(0, 0, 0, 0.05) 40%,
    rgba(0, 0, 0, 0.05) 60%,
    rgba(0, 0, 0, 0.22) 100%
  );
}
.bg-preview-bubble {
  max-width: 75%;
  padding: 8px 14px;
  border-radius: 14px;
  font-size: 12px;
  line-height: 1.5;
  backdrop-filter: blur(4px);
}
.bg-preview-bubble--user {
  align-self: flex-end;
  background: rgba(var(--accent-primary-rgb, 68, 130, 255), 0.7);
  color: #fff;
  border-bottom-right-radius: 4px;
}
.bg-preview-bubble--ai {
  align-self: flex-start;
  background: rgba(255, 255, 255, 0.7);
  color: #333;
  border-bottom-left-radius: 4px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}
.bg-hover-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.5);
  opacity: 0;
  transition: opacity 0.3s;
  border-radius: 12px;
}
.bg-preview-image:hover .bg-hover-overlay {
  opacity: 1;
}
.bg-hover-text {
  color: #fff;
  font-size: 15px;
  font-weight: 500;
  text-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}
.bg-preview-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 4px;
}
.bg-preview-label {
  font-size: 12px;
  color: var(--text-muted);
}

/* ===== 头像 ===== */
.avatar-row {
  display: flex;
  align-items: center;
  gap: 16px;
}
.avatar-preview {
  width: 56px;
  height: 56px;
  overflow: hidden;
  background: var(--bg-secondary);
  border: 2px solid var(--border-color);
  flex-shrink: 0;
}
.avatar-round {
  border-radius: 50%;
}
.avatar-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.avatar-btns {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

/* ===== 通用 ===== */
.slider-value {
  min-width: 48px;
  text-align: right;
  font-size: 13px;
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
}
</style>
