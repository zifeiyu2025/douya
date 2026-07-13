<template>
  <!-- 主题模式选择器（亮色/深色/跟随系统） -->
  <n-form-item label="主题模式">
    <n-radio-group :value="themeStore.mode" @update:value="handleModeChange">
      <n-radio value="light">亮色</n-radio>
      <n-radio value="dark">深色</n-radio>
      <n-radio value="auto">跟随系统</n-radio>
    </n-radio-group>
  </n-form-item>

  <!-- 外观设置 -->
  <n-form-item label="聊天背景">
    <div class="upload-wrapper">
        <div class="upload-placeholder" v-if="!formConfig.chat_background" @click="selectBackgroundImage">
          <div class="upload-icon">🖼️</div>
          <div class="upload-text">点击选择背景图片</div>
        </div>
        <div class="upload-preview" v-else @click="selectBackgroundImage">
          <img :src="backgroundImageUrl" class="background-preview" />
          <div class="hover-overlay">
            <span class="hover-hint">点击更改</span>
          </div>
          <div class="upload-actions">
            <n-button type="primary" size="small" ghost @click.stop="clearBackground">清除</n-button>
          </div>
        </div>
    </div>
  </n-form-item>

  <n-form-item label="背景透明度" v-if="formConfig.chat_background">
    <n-slider v-model:value="formConfig.chat_background_opacity" :min="0.2" :max="1" :step="0.05" />
    <span class="slider-value">{{ Math.round(formConfig.chat_background_opacity * 100) }}%</span>
  </n-form-item>

  <n-form-item label="用户头像">
    <div class="avatar-upload-wrapper">
      <div class="avatar-preview">
        <img :src="formConfig.user_avatar || defaultUserAvatar" class="avatar-img" />
      </div>
      <div class="avatar-buttons">
        <n-upload
          :show-file-list="false"
          :custom-request="(data: any) => handleAvatarUpload(data, 'user_avatar')"
          accept="image/*"
        >
          <n-button type="primary" size="small" ghost>上传</n-button>
        </n-upload>
        <n-button type="primary" size="small" ghost @click="clearUserAvatar" v-if="formConfig.user_avatar">清除</n-button>
      </div>
    </div>
  </n-form-item>

  <n-form-item label="AI头像">
    <div class="avatar-upload-wrapper">
      <div class="avatar-preview ai-avatar">
        <img :src="formConfig.ai_avatar || defaultAiAvatar" class="avatar-img" />
      </div>
      <div class="avatar-buttons">
        <n-upload
          :show-file-list="false"
          :custom-request="(data: any) => handleAvatarUpload(data, 'ai_avatar')"
          accept="image/*"
        >
          <n-button type="primary" size="small" ghost>上传</n-button>
        </n-upload>
        <n-button type="primary" size="small" ghost @click="clearAIAvatar" v-if="formConfig.ai_avatar">清除</n-button>
      </div>
    </div>
  </n-form-item>

  <!-- 系统提示词 -->
  <n-form-item>
    <template #label>系统提示词 <HelpTip content="追加在豆芽默认提示词后面的自定义指令，用于补充角色设定和行为约束。留空则仅使用默认提示词" /></template>
    <n-input v-model:value="formConfig.system_prompt" type="textarea" placeholder="自定义提示词将追加在豆芽默认提示词后面，用于补充角色设定和行为指令..." :autosize="{ minRows: 6, maxRows: 20 }" class="rounded-textarea" style="width: 100%;" @blur="autoSave" />
  </n-form-item>

  <!-- 推理配置 -->
  <n-form-item>
    <template #label>推理模式 <HelpTip content="控制模型的推理（思考）行为。on=始终开启推理，off=关闭推理，auto=由模型自行决定是否推理" /></template>
    <n-select v-model:value="formConfig.reasoning" :options="reasoningOptions" :disabled="!supportsReasoning" />
  </n-form-item>
  <n-text v-if="!supportsReasoning" depth="3" style="font-size: 12px; margin-top: -12px; display: block; margin-bottom: 8px;">
    当前模型不支持推理
  </n-text>
  <n-form-item>
    <template #label>推理预算 <HelpTip content="限制推理（思考）过程的 token 数量。-1=无限（由模型自行决定），0=立即结束推理，N>0=限制为 N 个 token" /></template>
    <n-input-number v-model:value="formConfig.reasoning_budget" :min="-1" :step="1" placeholder="-1" style="width: 100%" :disabled="!supportsReasoning || formConfig.backend_sampling" />
  </n-form-item>
  <n-text v-if="formConfig.backend_sampling && supportsReasoning" depth="3" style="font-size: 12px; margin-top: -12px; display: block; margin-bottom: 8px;">
    后端采样与推理预算不兼容，推理预算已禁用
  </n-text>
  <n-form-item v-if="formConfig.reasoning_budget > 0">
    <template #label>预算耗尽消息 <HelpTip content="推理预算耗尽时显示给用户的消息。留空则使用默认提示" /></template>
    <n-input v-model:value="formConfig.reasoning_budget_message" placeholder="推理预算耗尽时显示给用户的消息（留空使用默认提示）" @blur="autoSave" :disabled="!supportsReasoning" />
  </n-form-item>

  <!-- 生成参数（核心项） -->
  <div v-if="currentModelRef" class="model-ref-card">
    <div class="model-ref-header">
      <span class="model-ref-icon">📋</span>
      <span class="model-ref-title">{{ currentModelRef.name }} 官方参考参数</span>
      <span class="model-ref-current" v-if="settingsStore.currentModel">当前: {{ settingsStore.currentModel }}</span>
    </div>
    <div v-if="currentModelRef.raw_thinking" class="model-ref-tabs">
      <n-button
        :type="!refShowThinking ? 'primary' : 'default'"
        ghost
        size="small"
        class="model-ref-tab"
        :class="{ active: !refShowThinking }"
        @click="refShowThinking = false"
      >非思考模式</n-button>
      <n-button
        :type="refShowThinking ? 'primary' : 'default'"
        ghost
        size="small"
        class="model-ref-tab"
        :class="{ active: refShowThinking }"
        @click="refShowThinking = true"
      >思考模式</n-button>
    </div>
    <div class="model-ref-body">
      <template v-if="!refShowThinking">
        <div class="model-ref-row" v-for="item in currentModelRef.params" :key="item.label">
          <span class="model-ref-label">{{ item.label }}</span>
          <span class="model-ref-value">{{ item.value }}</span>
        </div>
      </template>
      <template v-else-if="currentModelRef.params_thinking">
        <div class="model-ref-row" v-for="item in currentModelRef.params_thinking" :key="item.label">
          <span class="model-ref-label">{{ item.label }}</span>
          <span class="model-ref-value">{{ item.value }}</span>
        </div>
      </template>
      <div v-if="currentModelRef.note" class="model-ref-note">{{ currentModelRef.note }}</div>
    </div>
    <n-button type="primary" size="small" ghost class="model-ref-apply" @click="applyModelRef">
      应用参考参数
    </n-button>
  </div>

  <n-form-item>
    <template #label>温度 <HelpTip content="控制回答的随机性。值越低越确定保守，值越高越多样创意。一般 0.3-0.8 之间" /></template>
    <n-slider v-model:value="formConfig.temperature" :min="0" :max="2" :step="0.01" />
    <span class="slider-value">{{ formConfig.temperature }}</span>
    <n-button v-if="currentModelRef" type="primary" size="small" ghost class="reset-btn" @click="formConfig.temperature = activeModelRefRaw.temperature">{{ activeModelRefRaw.temperature }}</n-button>
  </n-form-item>
  <n-form-item>
    <template #label>Top P <HelpTip content="从概率最高的候选词中筛选，只考虑累计概率达到此阈值的词。0.95 表示保留前 95% 概率的词" /></template>
    <n-slider v-model:value="formConfig.top_p" :min="0" :max="1" :step="0.01" />
    <span class="slider-value">{{ formConfig.top_p }}</span>
    <n-button v-if="currentModelRef" type="primary" size="small" ghost class="reset-btn" @click="formConfig.top_p = activeModelRefRaw.top_p">{{ activeModelRefRaw.top_p }}</n-button>
  </n-form-item>
  <n-form-item>
    <template #label>Top K <HelpTip content="只从概率最高的 K 个候选词中选择。值越小选择越少越确定，0 表示不限制" /></template>
    <n-slider v-model:value="formConfig.top_k" :min="0" :max="100" :step="1" />
    <span class="slider-value">{{ formConfig.top_k }}</span>
    <n-button v-if="currentModelRef" type="primary" size="small" ghost class="reset-btn" @click="formConfig.top_k = activeModelRefRaw.top_k">{{ activeModelRefRaw.top_k }}</n-button>
  </n-form-item>
  <n-form-item>
    <template #label>上下文长度 <HelpTip content="模型能记住的对话历史 token 数。值越大记忆越长但占用显存越多，超过模型支持的最大值会被自动截断" /></template>
    <n-slider v-model:value="contextSizeIndex" :min="0" :max="contextSizeSteps.length - 1" :step="1" :marks="contextSizeMarks" />
    <span class="slider-value">{{ formatContextSize(formConfig.context_size) }}</span>
    <n-button v-if="currentModelRef" type="primary" size="small" ghost class="reset-btn" @click="applyContextSizeRef">{{ formatContextSize(activeModelRefRaw.context_size) }}</n-button>
  </n-form-item>
  <n-form-item>
    <template #label>重复惩罚 <HelpTip content="大于 1 时惩罚重复内容，防止 AI 反复说同样的话。1.0 表示不惩罚" /></template>
    <n-slider v-model:value="formConfig.repeat_penalty" :min="0" :max="2" :step="0.01" />
    <span class="slider-value">{{ formConfig.repeat_penalty }}</span>
  </n-form-item>
  <n-form-item>
    <template #label>Min-P <HelpTip content="根据最高概率词动态过滤低概率词。0.05 表示过滤掉概率不到最高词 5% 的候选词" /></template>
    <n-slider v-model:value="formConfig.min_p" :min="0" :max="1" :step="0.01" />
    <span class="slider-value">{{ formConfig.min_p }}</span>
  </n-form-item>
  <n-form-item>
    <template #label>DRY 采样倍数 <HelpTip content="防止 AI 重复相同句式。0 表示关闭，大于 0 时值越强越不容易重复" /></template>
    <n-slider v-model:value="formConfig.dry_multiplier" :min="0" :max="5" :step="0.01" />
    <span class="slider-value">{{ formConfig.dry_multiplier }}</span>
  </n-form-item>
  <n-form-item v-if="formConfig.dry_multiplier > 0">
    <template #label>DRY 基准值 <HelpTip content="DRY 采样的基础惩罚倍数。值越大对重复句式的惩罚越强，通常 1.0-2.0" /></template>
    <n-slider v-model:value="formConfig.dry_base" :min="1" :max="3" :step="0.01" />
    <span class="slider-value">{{ formConfig.dry_base }}</span>
  </n-form-item>
  <n-form-item v-if="formConfig.dry_multiplier > 0">
    <template #label>DRY 允许长度 <HelpTip content="允许重复的 token 序列长度。短于此长度的重复不会被惩罚，值越小越严格" /></template>
    <n-slider v-model:value="formConfig.dry_allowed_length" :min="1" :max="10" :step="1" />
    <span class="slider-value">{{ formConfig.dry_allowed_length }}</span>
  </n-form-item>
  <n-form-item v-if="formConfig.dry_multiplier > 0">
    <template #label>序列中断符 <HelpTip content="Dry 采样的序列中断符，遇到这些字符时重置 Dry 惩罚。多个中断符用逗号分隔，如 \n,。,！,？" /></template>
    <n-input v-model:value="formConfig.dry_sequence_breaker" placeholder="如：\n,。,？（逗号分隔）" @blur="autoSave" />
  </n-form-item>
  <n-form-item v-if="formConfig.dry_multiplier > 0">
    <template #label>惩罚窗口 <HelpTip content="Dry 惩罚的回看窗口大小（token 数）。0=使用 repeat_last_n 的值" /></template>
    <n-input-number v-model:value="formConfig.dry_penalty_last_n" :min="0" :max="2048" :step="1" placeholder="0" style="width: 100%" @blur="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>自定义采样器顺序 <HelpTip content="手动指定采样器的执行顺序，多个采样器用逗号分隔，如 top_k,top_p,temperature。留空则使用后端默认顺序" /></template>
    <n-input v-model:value="formConfig.samplers" placeholder="如：top_k,top_p,temperature（留空使用默认）" @blur="autoSave" />
  </n-form-item>
  <n-form-item label="忽略 EOS">
    <n-checkbox v-model:checked="formConfig.ignore_eos" @update:value="autoSave">
      忽略 EOS 继续生成
    </n-checkbox>
    <span class="setting-hint">开启后即使遇到结束符也继续生成，可用于强制生成长文本</span>
  </n-form-item>
  <n-form-item>
    <template #label>自适应采样目标 <HelpTip content="请求级自适应采样的目标熵值（0-1）。0=禁用，设置后模型会动态调整采样参数以逼近该目标" /></template>
    <n-input-number v-model:value="formConfig.adaptive_target" :min="0" :max="1" :step="0.01" placeholder="0" style="width: 100%" @blur="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>自适应采样衰减 <HelpTip content="请求级自适应采样的衰减系数（0-1）。0=禁用，控制自适应调整的收敛速度，值越小调整越平缓" /></template>
    <n-input-number v-model:value="formConfig.adaptive_decay" :min="0" :max="1" :step="0.01" placeholder="0" style="width: 100%" @blur="autoSave" />
  </n-form-item>

  <div class="gen-params-save-row">
    <span class="gen-params-status" v-if="genParamsDirty">设置已修改，自动保存中...</span>
    <span class="gen-params-status saved" v-else-if="formConfig.context_size > 0">✓ 已保存</span>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import {
  NInputNumber, NSelect, NCheckbox,
} from 'naive-ui'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import { useThemeStore } from '../../stores/theme'
// F-1.1/F-1.2：抽取为公共组件，消除三处重复定义
import HelpTip from '../ui/HelpTip.vue'

const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)!

const {
  formConfig, autoSave, genParamsDirty,
  backgroundImageUrl, selectBackgroundImage, clearBackground,
  handleAvatarUpload, clearUserAvatar,
  clearAIAvatar,
  defaultUserAvatar, defaultAiAvatar,
  reasoningOptions, supportsReasoning,
  currentModelRef, activeModelRefRaw, refShowThinking, applyModelRef,
  contextSizeIndex, contextSizeSteps, contextSizeMarks,
  formatContextSize, applyContextSizeRef,
  settingsStore,
} = ctx

// 主题模式选择器：从 themeStore 获取 mode，调用 setMode 切换
const themeStore = useThemeStore()

function handleModeChange(value: 'light' | 'dark' | 'auto') {
  themeStore.setMode(value)
}
</script>

<style scoped>
.upload-wrapper {
  width: 100%;
}

.upload-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 32px;
  border: 2px dashed var(--border-color);
  border-radius: var(--border-radius-md);
  cursor: pointer;
  transition: all 0.2s;
}

.upload-placeholder:hover {
  border-color: var(--accent-primary);
  background: var(--accent-tertiary);
}

.upload-icon {
  font-size: 32px;
}

.upload-text {
  color: var(--text-secondary);
}

.upload-preview {
  position: relative;
  border-radius: var(--border-radius-md);
  overflow: hidden;
  cursor: pointer;
}

.background-preview {
  width: 100%;
  height: 160px;
  object-fit: cover;
  border-radius: var(--border-radius-md);
}

.upload-actions {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  padding: 8px;
  background: linear-gradient(transparent, rgba(0, 0, 0, 0.5));
  display: flex;
  justify-content: center;
  z-index: 2;
}

.hover-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.35s cubic-bezier(0.4, 0, 0.2, 1);
  pointer-events: none;
  border-radius: var(--border-radius-md);
  z-index: 1;
}

.upload-preview:hover .hover-overlay {
  opacity: 1;
}

.hover-hint {
  color: #ffffff;
  font-size: 18px;
  font-weight: 500;
  letter-spacing: 0.05em;
  text-shadow: 0 1px 4px rgba(0, 0, 0, 0.35);
  user-select: none;
}

:deep(.upload-actions .n-button.n-button--ghost) {
  color: #ffffff;
  border-color: rgba(255, 255, 255, 0.6);
}

:deep(.upload-actions .n-button.n-button--ghost:hover) {
  color: #ffffff;
  border-color: #ffffff;
  background: rgba(255, 255, 255, 0.15);
}

.avatar-upload-wrapper {
  display: flex;
  align-items: center;
  gap: 16px;
}

.avatar-buttons {
  display: flex;
  align-items: center;
  gap: 8px;
}

.avatar-preview {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  box-shadow: var(--shadow-md);
  transition: all 0.2s;
  flex-shrink: 0;
  background: var(--bg-secondary);
}

.avatar-preview:hover {
  transform: scale(1.05);
  box-shadow: var(--shadow-lg);
}

.avatar-preview.ai-avatar {
}

.avatar-preview .avatar-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: 50%;
  aspect-ratio: 1;
}

.slider-value {
  min-width: 48px;
  text-align: right;
  font-size: 13px;
  color: var(--text-secondary);
  margin-left: 12px;
}

.rounded-textarea :deep(.n-input__textarea-wrapper) {
  border-radius: var(--border-radius-lg);
  /* 透明，让 n-input 容器底色（Input.color = --bg-input）透出，避免双底色 */
  background: transparent;
}

.rounded-textarea :deep(.n-input__textarea) {
  border-radius: var(--border-radius-lg);
  /* 透明，与 wrapper 一致，由 n-input 容器统一提供底色 */
  background: transparent;
}

.rounded-textarea :deep(.n-input__border) {
  border-radius: var(--border-radius-lg);
}

.rounded-textarea :deep(.n-input__state-border) {
  border-radius: var(--border-radius-lg);
}

.reset-btn {
  margin-left: 4px;
  min-width: 32px;
}

.model-ref-card {
  margin-bottom: 16px;
  border-radius: var(--border-radius-md);
  border: 1px solid var(--border-color);
  background: var(--bg-secondary);
  overflow: hidden;
}

.model-ref-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-tertiary, var(--bg-primary));
}

.model-ref-icon {
  font-size: 14px;
}

.model-ref-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.model-ref-current {
  margin-left: auto;
  font-size: 11px;
  color: var(--text-muted);
  font-weight: 400;
}

.model-ref-body {
  padding: 10px 14px;
}

.model-ref-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 0;
  font-size: 12px;
}

.model-ref-label {
  color: var(--text-secondary);
}

.model-ref-value {
  color: var(--text-primary);
  font-weight: 500;
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
}

.model-ref-note {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--border-color);
  font-size: 11px;
  color: var(--text-muted);
  line-height: 1.5;
}

.model-ref-apply {
  width: 100%;
  border-radius: 0 0 var(--border-radius-md) var(--border-radius-md);
}

.model-ref-tabs {
  display: flex;
  border-bottom: 1px solid var(--border-color);
  padding: 0 14px;
}

.model-ref-tab {
  border-radius: 0;
}

.model-ref-tab.active {
  font-weight: 600;
}

.gen-params-save-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  padding: 12px 0 4px;
}

.gen-params-status {
  font-size: 12px;
  color: var(--accent-warning);
}

.gen-params-status.saved {
  color: var(--accent-success);
}

/* F-1.2：.help-tip-icon 样式已抽取到 ui/HelpTip.vue，此处不再重复定义 */

.setting-hint {
  font-size: 12px;
  color: var(--n-text-color-3);
  margin-left: 12px;
}
</style>
