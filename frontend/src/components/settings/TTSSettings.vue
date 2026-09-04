<!--
  TTSSettings: TTS 朗读设置组件

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
          <HelpTip
            content="有网时优先使用微软在线神经语音，音质更自然；无网络时自动回退到本地语音"
          />
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
            content="开启「在线 TTS」时仅列出云端当前可用的发音人（不可用的会被云端拒绝并自动回退默认晓晓，听感不变）；关闭在线时列出系统全部本地发音人。选「自动」则按优先级自动挑选"
          />
        </template>
        <n-select
          v-model:value="formConfig.tts_voice"
          :options="voiceOptions"
          placeholder="自动挑选（推荐晓晓）"
          clearable
          :loading="!tts.isSupported.value || voices.length === 0"
          :render-label="renderVoiceLabel as unknown as (option: SelectOption) => VNodeChild"
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
import { computed, ref, watch, onMounted, onUnmounted, createVNode } from 'vue'
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
// 域切片：TTS 仅需核心表单与保存
const { core } = ctx
const { formConfig, autoSave } = core

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
 * 这是完整目录，渲染时会按 CLOUD_AVAILABLE_VOICES 过滤，只展示云端当前可用的。
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
 * 当前云端（微软 Edge TTS）真正可用的中文 Neural 音色清单。
 * 实测（2026-09-04）：22 个中文发音人中仅以下 6 个可用；
 * 其余（慧慧/瑶瑶/康康/晓辰/晓涵/晓梦/晓墨/晓秋/晓睿/晓双/晓悠/晓甄/云枫/云浩/云野/云泽）
 * 会被云端以 Unsupported voice 拒绝。若允许选择，后端会静默回退默认晓晓，
 * 导致"切换了发音人但听到的总是同一个人"。
 * 云端可用性随地区/环境变化，未来需要增删时改这里即可（value 用微软 Neural 音色名）。
 */
const CLOUD_AVAILABLE_VOICES = new Set<string>([
  'zh-CN-XiaoxiaoNeural', // 晓晓（女，默认）
  'zh-CN-YunxiNeural', // 云希（男）
  'zh-CN-YunyangNeural', // 云扬（男）
  'zh-CN-XiaoyiNeural', // 晓伊（女）
  'zh-CN-YunjianNeural', // 云健（男，仅在线）
  'zh-CN-YunxiaNeural' // 云霞（女，仅在线）
])

/**
 * 本地发音人名 → 微软在线 Neural 音色名 映射（与后端 ResolveOnlineVoice 一致）。
 * 在线模式下列表过滤用：本地发音人只有映射后落在 CLOUD_AVAILABLE_VOICES 里才可选。
 */
const LOCAL_TO_ONLINE: Record<string, string> = {
  'Microsoft Xiaoxiao': 'zh-CN-XiaoxiaoNeural',
  'Microsoft Yunxi': 'zh-CN-YunxiNeural',
  'Microsoft Yunyang': 'zh-CN-YunyangNeural',
  'Microsoft Xiaoyi': 'zh-CN-XiaoyiNeural',
  'Microsoft Huihui': 'zh-CN-HuihuiNeural',
  'Microsoft Yaoyao': 'zh-CN-YaoyaoNeural',
  'Microsoft Kangkang': 'zh-CN-KangkangNeural'
}

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
    children.push(
      createVNode(
        NTag,
        { size: 'small', type: 'success', bordered: false, round: true },
        { default: () => '在线' }
      )
    )
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
 * 在线模式：直接列出云端当前可用的发音人（硬编码，不依赖浏览器本地语音列表，
 * 因为本机 Web Speech 可能未加载微软本地音色，导致晓晓/云希/云扬/晓伊不出现）；
 * 本地模式：列出浏览器全部本地发音人，中文语音排前，其余按优先级排序。
 */
const voiceOptions = computed<VoiceOption[]>(() => {
  if (!tts.isSupported.value) return []

  // ===== 在线模式：固定列出云端可用发音人 =====
  if (formConfig.value.tts_online) {
    const options: VoiceOption[] = [
      {
        label: '自动（推荐晓晓）',
        value: ''
      }
    ]
    // 本地+在线兼有的 4 个：value 用本地发音人名，在线合成时后端自动映射为对应 Neural 音色
    const localCloudVoices = [
      { label: '微软晓晓（在线）', value: 'Microsoft Xiaoxiao' },
      { label: '微软云希（在线）', value: 'Microsoft Yunxi' },
      { label: '微软云扬（在线）', value: 'Microsoft Yunyang' },
      { label: '微软晓伊（在线）', value: 'Microsoft Xiaoyi' }
    ]
    for (const v of localCloudVoices) {
      options.push({
        label: v.label,
        value: v.value,
        onlineCapable: true
      })
    }
    // 仅在线发音人（云健/云霞）：value 用 Neural 名，后端原样透传
    for (const ov of ONLINE_ONLY_VOICES) {
      if (CLOUD_AVAILABLE_VOICES.has(ov.name)) {
        options.push({
          label: ov.label,
          value: ov.name,
          onlineCapable: true
        })
      }
    }
    return options
  }

  // ===== 本地模式：列出浏览器全部本地发音人 =====
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

// 在线模式下列表被过滤：若当前选中的发音人已不在可选列表（例如之前选了
// 康康/云枫等当前云端不可用的音色，或云端可用性变化），自动回到「自动」，
// 避免下拉框显示一个不存在的选项、朗读时又被静默回退晓晓。
watch(
  () => [formConfig.value.tts_voice, formConfig.value.tts_online],
  () => {
    if (!formConfig.value.tts_online) return
    const cur = formConfig.value.tts_voice
    if (!cur) return
    // 本地发音人先映射成在线音色名；仅在线发音人名本身就是 Neural 名
    const onlineName = LOCAL_TO_ONLINE[cur] ?? cur
    if (!CLOUD_AVAILABLE_VOICES.has(onlineName)) {
      formConfig.value.tts_voice = ''
      syncConfigToTTS()
      void autoSave()
    }
  },
  { immediate: true }
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
