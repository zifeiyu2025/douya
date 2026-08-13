<!--
  TTSSettings: TTS 朗读设置组件
  生活类比：像"播音员调度台"的控制面板——
    - 选哪个播音员念（发音人下拉）
    - 念多快（语速滑块）
    - 声音多高（音调滑块）
    - 多大声（音量滑块）
    - 试听效果（试听按钮）

  设计要点：
    - 发音人列表从浏览器 SpeechSynthesis API 实时获取
    - 中文语音排前，按微软自然语音优先级排序
    - 选"自动"则由 useTTS 按优先级自动挑选（晓晓→云希→...）
    - 试听按钮用当前配置朗读一段示例文字
    - 所有改动通过 autoSave 持久化到后端 config
-->
<template>
  <div class="tts-settings">
    <!-- 启用开关 -->
    <n-form-item label="启用朗读">
      <div class="toggle-row">
        <n-switch v-model:value="formConfig.tts_enabled" @update:value="onToggleEnabled" />
        <span class="toggle-hint">
          {{ formConfig.tts_enabled ? '已启用，AI 消息底部显示朗读按钮' : '已关闭，朗读按钮隐藏' }}
        </span>
      </div>
    </n-form-item>

    <template v-if="formConfig.tts_enabled">
      <!-- 在线 TTS 开关 -->
      <n-form-item>
        <template #label>
          在线 TTS（微软云端）
          <HelpTip content="有网时优先使用微软在线神经语音，音质更自然；无网络时自动回退到本地语音" />
        </template>
        <div class="toggle-row">
          <n-switch v-model:value="formConfig.tts_online" @update:value="onToggleOnline" />
          <span class="toggle-hint">
            {{ formConfig.tts_online ? '有网用在线云端语音，无网回退本地' : '始终使用本地语音' }}
          </span>
        </div>
      </n-form-item>

      <!-- 发音人选择 -->
      <n-form-item>
        <template #label>
          发音人
          <HelpTip
            content="选择系统可用的发音人。标注「在线」的发音人在启用在线 TTS 时会使用微软云端神经语音，音质更佳。选「自动」则按优先级自动挑选"
          />
        </template>
        <n-select
          v-model:value="formConfig.tts_voice"
          :options="voiceOptions"
          placeholder="自动挑选（推荐晓晓）"
          clearable
          :loading="!tts.isSupported.value || voices.length === 0"
          :render-label="(renderVoiceLabel as unknown as (option: SelectOption) => VNodeChild)"
          @update:value="onVoiceChange"
        />
      </n-form-item>

      <!-- 语速 -->
      <n-form-item>
        <template #label>
          语速
          <HelpTip content="0.5=慢速朗读，1.0=正常速度，2.0=快速朗读" />
        </template>
        <div class="slider-row">
          <n-slider
            v-model:value="formConfig.tts_rate"
            :min="0.5"
            :max="2.0"
            :step="0.1"
            :marks="{ 0.5: '慢', 1.0: '正常', 1.5: '快', 2.0: '极快' }"
            @update:value="onParamChange"
          />
          <span class="slider-value">{{ formConfig.tts_rate.toFixed(1) }}x</span>
        </div>
      </n-form-item>

      <!-- 音调 -->
      <n-form-item>
        <template #label>
          音调
          <HelpTip content="0=低沉，1.0=正常，2=尖锐。调整声音的高低音" />
        </template>
        <div class="slider-row">
          <n-slider
            v-model:value="formConfig.tts_pitch"
            :min="0"
            :max="2"
            :step="0.1"
            :marks="{ 0: '低', 1: '正常', 2: '高' }"
            @update:value="onParamChange"
          />
          <span class="slider-value">{{ formConfig.tts_pitch.toFixed(1) }}</span>
        </div>
      </n-form-item>

      <!-- 音量 -->
      <n-form-item>
        <template #label>
          音量
          <HelpTip content="0=静音，1.0=最大音量" />
        </template>
        <div class="slider-row">
          <n-slider
            v-model:value="formConfig.tts_volume"
            :min="0"
            :max="1"
            :step="0.05"
            :marks="{ 0: '静音', 0.5: '中', 1: '最大' }"
            @update:value="onParamChange"
          />
          <span class="slider-value">{{ Math.round(formConfig.tts_volume * 100) }}%</span>
        </div>
      </n-form-item>

      <!-- 试听按钮 -->
      <n-form-item label="试听效果">
        <n-button type="primary" ghost :disabled="!tts.isSupported.value" @click="previewVoice">
          <template #icon>
            <n-icon><VolumeLowOutline /></n-icon>
          </template>
          {{ isPreviewing ? '停止试听' : '试听当前配置' }}
        </n-button>
        <span v-if="!tts.isSupported.value" class="unsupported-hint">当前浏览器不支持语音合成</span>
      </n-form-item>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted, h, createVNode } from 'vue'
import type { VNodeChild } from 'vue'
import { NFormItem, NSwitch, NSelect, NSlider, NButton, NIcon, NTag, useMessage } from 'naive-ui'
import type { SelectOption } from 'naive-ui'
import { VolumeLowOutline } from '@vicons/ionicons5'
import { inject } from 'vue'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import { useTTS } from '../../composables/useTTS'
import HelpTip from '../ui/HelpTip.vue'

defineOptions({ name: 'TTSSettings' })

// 空值保护：如果组件被误用在 SettingsView 之外，给出明确错误而非 undefined 崩溃
const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)
if (!ctx) {
  throw new Error('TTSSettings 必须在 SettingsView 内使用（缺少 settingsContext provide）')
}
const { formConfig, autoSave } = ctx

const message = useMessage()
const tts = useTTS()
const voices = tts.voices
const isPreviewing = ref(false)
// 试听结束轮询定时器（仅用于驱动"停止试听"按钮状态切换），组件卸载时清理
let previewTimer: ReturnType<typeof setInterval> | null = null

/** 清理试听轮询定时器 */
function clearPreviewTimer(): void {
  if (previewTimer) {
    clearInterval(previewTimer)
    previewTimer = null
  }
}

// ===== 发音人选项构建 =====

/**
 * 中文语音偏好排序（与 useTTS 内部一致，仅用于 UI 排序）
 */
const CN_PREFERENCE_ORDER = [
  'Microsoft Xiaoxiao',
  'Microsoft Yunxi',
  'Microsoft Yunyang',
  'Microsoft Xiaoyi',
  'Microsoft Huihui',
  'Microsoft Yaoyao',
  'Microsoft Kangkang'
]

/**
 * 仅在线（云端 Neural）的中文发音人：Windows 本地 Web Speech 不提供，
 * 仅在启用「在线 TTS」时可用。value 直接传在线 Neural 音色名给后端，
 * 后端 ResolveOnlineVoice 会原样透传，从而选定这些本地没有的音色。
 */
const ONLINE_ONLY_VOICES = [
  { name: 'zh-CN-XiaochenNeural', label: '微软晓辰（在线）', gender: '男' },
  { name: 'zh-CN-XiaohanNeural', label: '微软晓涵（在线）', gender: '女' },
  { name: 'zh-CN-XiaomengNeural', label: '微软晓梦（在线）', gender: '女' },
  { name: 'zh-CN-XiaomoNeural', label: '微软晓墨（在线）', gender: '女' },
  { name: 'zh-CN-XiaoqiuNeural', label: '微软晓秋（在线）', gender: '女' },
  { name: 'zh-CN-XiaoruiNeural', label: '微软晓睿（在线）', gender: '女' },
  { name: 'zh-CN-XiaoshuangNeural', label: '微软晓双（在线）', gender: '女' },
  { name: 'zh-CN-XiaoyouNeural', label: '微软晓悠（在线）', gender: '女' },
  { name: 'zh-CN-XiaozhenNeural', label: '微软晓甄（在线）', gender: '女' },
  { name: 'zh-CN-YunfengNeural', label: '微软云枫（在线）', gender: '男' },
  { name: 'zh-CN-YunhaoNeural', label: '微软云浩（在线）', gender: '男' },
  { name: 'zh-CN-YunjianNeural', label: '微软云健（在线）', gender: '男' },
  { name: 'zh-CN-YunxiaNeural', label: '微软云霞（在线）', gender: '女' },
  { name: 'zh-CN-YunyeNeural', label: '微软云野（在线）', gender: '男' },
  { name: 'zh-CN-YunzeNeural', label: '微软云泽（在线）', gender: '男' }
]

/**
 * 判断本地发音人是否可在线使用（能映射到微软云端 Neural 语音）
 * 与 CN_PREFERENCE_ORDER 中的微软语音一致
 */
function isOnlineCapable(name: string): boolean {
  const lower = (name || '').toLowerCase()
  return CN_PREFERENCE_ORDER.some(p => lower.includes(p.toLowerCase()))
}

/**
 * 渲染发音人下拉选项标签（含「在线」标记）
 * 使用 Naive UI n-select 的 render-label 属性（函数形式），非插槽
 */
function renderVoiceLabel(option: VoiceOption): VNodeChild {
  const children: VNodeChild[] = [createVNode('span', {}, option.label || '')]
  if (option.onlineCapable) {
    children.push(createVNode(NTag, { size: 'small', type: 'success', bordered: false, round: true }, { default: () => '在线' }))
  }
  return createVNode('span', { class: 'voice-option' }, children)
}

/**
 * 发音人选项（扩展 Naive UI SelectOption，携带 onlineCapable 标记）
 */
interface VoiceOption extends SelectOption {
  onlineCapable?: boolean
}

/**
 * 构建发音人下拉选项
 * 生活类比：把系统里所有"播音员"列成菜单，中文的排前面，按知名度排序。
 */
const voiceOptions = computed<VoiceOption[]>(() => {
  if (!tts.isSupported.value) return []

  // 第一项：自动（空值，让 useTTS 按优先级挑选）
  const options: VoiceOption[] = [
    {
      label: '自动（推荐晓晓）',
      value: ''
    }
  ]

  // 筛选并排序中文语音
  const cnVoices = voices.value.filter(v => v.lang?.toLowerCase().startsWith('zh'))
  const sortedCnVoices = [...cnVoices].sort((a, b) => {
    const ia = CN_PREFERENCE_ORDER.findIndex(p => a.name?.includes(p))
    const ib = CN_PREFERENCE_ORDER.findIndex(p => b.name?.includes(p))
    // 偏好表里的排前面（ia/ib 为 -1 表示不在偏好表，放后面）
    if (ia !== -1 && ib !== -1) return ia - ib
    if (ia !== -1) return -1
    if (ib !== -1) return 1
    return a.name.localeCompare(b.name)
  })

  for (const v of sortedCnVoices) {
    options.push({
      label: `${v.name} (${v.lang})`,
      value: v.name,
      onlineCapable: isOnlineCapable(v.name)
    })
  }

  // 追加仅在线（云端 Neural）的中文发音人：本地没有，但可在线使用
  for (const ov of ONLINE_ONLY_VOICES) {
    options.push({
      label: ov.label,
      value: ov.name,
      onlineCapable: true
    })
  }

  // 其他语言语音放最后（灰色分组）
  const otherVoices = voices.value.filter(v => !v.lang?.toLowerCase().startsWith('zh'))
  if (otherVoices.length > 0) {
    options.push({
      label: '—— 其他语言 ——',
      value: '__other_group__'
    })
    for (const v of otherVoices) {
      options.push({
        label: `${v.name} (${v.lang})`,
        value: v.name,
        onlineCapable: isOnlineCapable(v.name)
      })
    }
  }

  return options
})

// ===== 同步配置到 useTTS =====

/**
 * 把 formConfig 中的 TTS 配置同步到 useTTS
 * 生活类比：把"设置面板"上的旋钮值告诉"调度台"。
 */
function syncConfigToTTS() {
  tts.updateConfig({
    voice: formConfig.value.tts_voice,
    rate: formConfig.value.tts_rate,
    pitch: formConfig.value.tts_pitch,
    volume: formConfig.value.tts_volume,
    online: formConfig.value.tts_online
  })
}

// 组件挂载时同步一次
onMounted(() => {
  syncConfigToTTS()
})

// formConfig 变化时同步（避免外部修改配置后 useTTS 不更新）
watch(
  () => [
    formConfig.value.tts_voice,
    formConfig.value.tts_rate,
    formConfig.value.tts_pitch,
    formConfig.value.tts_volume,
    formConfig.value.tts_online
  ],
  () => syncConfigToTTS(),
  { deep: true }
)

// ===== 事件处理 =====

/** 启用开关变化 */
async function onToggleEnabled() {
  await autoSave()
  if (formConfig.value.tts_enabled) {
    message.success('已启用朗读功能', { duration: 2000 })
  } else {
    // 关闭时停止当前朗读
    tts.stop()
    message.info('已关闭朗读功能', { duration: 2000 })
  }
}

/** 在线 TTS 开关变化 */
async function onToggleOnline() {
  await autoSave()
  // 切换在线/本地后立即停止当前朗读，避免后台在线音频继续播放
  tts.stop()
}

/** 发音人变化 */
async function onVoiceChange() {
  syncConfigToTTS()
  await autoSave()
}

/** 参数（语速/音调/音量）变化 */
async function onParamChange() {
  syncConfigToTTS()
  await autoSave()
}

/** 试听当前配置：自报发音人名字，方便用户辨识 */
function previewVoice() {
  // 试听中再点 → 停止
  if (isPreviewing.value) {
    clearPreviewTimer()
    tts.stop()
    isPreviewing.value = false
    return
  }
  // 构建含发音人名字的试听文本
  const voiceName = formConfig.value.tts_voice || '自动'
  const modeText = formConfig.value.tts_online ? '在线云端语音' : '本地语音'
  const previewText = `你好，我是豆芽的语音助手。当前使用的发音人是${voiceName}，模式为${modeText}。这是一段试听文字，用来检查发音人和参数设置是否满意。`
  tts.speak(previewText, '__preview__')
  isPreviewing.value = true
  // 监听朗读结束：朗读完毕（后端 onend 复位状态）后切回"试听"按钮
  clearPreviewTimer()
  previewTimer = setInterval(() => {
    if (!tts.isSpeaking('__preview__')) {
      isPreviewing.value = false
      clearPreviewTimer()
    }
  }, 200)
  // 30 秒后强制清理（兜底，防止极端情况下轮询泄漏）
  setTimeout(() => clearPreviewTimer(), 30000)
}

// 组件卸载时清理试听轮询定时器，避免泄漏
onUnmounted(() => {
  clearPreviewTimer()
})
</script>

<style scoped>
.tts-settings {
  width: 100%;
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.toggle-hint {
  font-size: 13px;
  color: var(--text-muted);
}

.slider-row {
  display: flex;
  align-items: center;
  gap: 16px;
  width: 100%;
}

.slider-row :deep(.n-slider) {
  flex: 1;
}

.slider-value {
  min-width: 50px;
  text-align: right;
  font-size: 13px;
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
}

.unsupported-hint {
  margin-left: 12px;
  font-size: 13px;
  color: var(--accent-warning);
}

.voice-option {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
</style>
