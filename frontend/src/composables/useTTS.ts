/**
 * useTTS.ts — 文本转语音（TTS）composable
 *
 * 生活类比：这个文件就像豆芽的"播音员调度员"——
 *   - 它知道系统里有哪些"播音员"（voices）
 *   - 它会按优先级挑最合适的播音员（Win11 的 Xiaoyi 优先）
 *   - 它负责安排播音员什么时候开始、暂停、停止
 *
 * 基于 W3C Web Speech API 的 SpeechSynthesis 接口实现。
 * WebView2（基于 Edge Chromium）完整支持此 API。
 *
 * 中文语音优先级（从高到低）：
 *   1. Microsoft Xiaoxiao（Win11 晓晓，最自然女声）        ← 首选
 *   2. Microsoft Yunxi  （Win11 云希，自然男声）           ← 次选
 *   3. Microsoft Yunyang（Win11 云扬，自然男声）           ← 次选
 *   4. Microsoft Xiaoyi （Win11 晓伊，自然女声）           ← 次选
 *   5. Microsoft Huihui （Win10 默认中文女声）             ← 兜底
 *   6. Microsoft Yaoyao （Win10 默认中文女声）             ← 兜底
 *   7. Microsoft Kangkang（Win10 默认中文男声）            ← 兜底
 *   8. 任意 zh-CN 语音                                    ← 最后兜底
 *   9. 任意语音                                           ← 实在没有中文就用任何能发音的
 */
import { ref, computed } from 'vue'

// ===== 类型定义 =====
// 浏览器原生 API 的类型补充（部分 TS 版本未内置）

interface SpeechSynthesisVoice {
  voiceURI: string
  name: string
  lang: string
  localService: boolean
  default: boolean
}

interface _SpeechSynthesisUtteranceInit {
  text: string
  lang?: string
  voice?: SpeechSynthesisVoice
  rate?: number
  pitch?: number
  volume?: number
}

// SpeechSynthesisUtterance 构造器（浏览器原生，部分 TS 版本未内置类型）
interface SpeechSynthesisUtterance {
  text: string
  lang: string
  voice: SpeechSynthesisVoice | null
  rate: number
  pitch: number
  volume: number
  onstart: ((this: SpeechSynthesisUtterance, ev: Event) => void) | null
  onend: ((this: SpeechSynthesisUtterance, ev: Event) => void) | null
  onerror: ((this: SpeechSynthesisUtterance, ev: SpeechSynthesisErrorEvent) => void) | null
  onpause: ((this: SpeechSynthesisUtterance, ev: Event) => void) | null
  onresume: ((this: SpeechSynthesisUtterance, ev: Event) => void) | null
}

interface SpeechSynthesisUtteranceConstructor {
  new (text?: string): SpeechSynthesisUtterance
}

interface SpeechSynthesisErrorEvent extends Event {
  error: string
}

// 通过 Window 接口访问原生 API（避免全局类型缺失）
interface Window {
  speechSynthesis: {
    speak(utterance: SpeechSynthesisUtterance): void
    cancel(): void
    pause(): void
    resume(): void
    getVoices(): SpeechSynthesisVoice[]
    speaking: boolean
    paused: boolean
    pending: boolean
    onvoiceschanged: ((this: Window, ev: Event) => void) | null
  }
  SpeechSynthesisUtterance: SpeechSynthesisUtteranceConstructor
}

// ===== 模块级单例状态 =====
// 生活类比：整个应用共用一个"播音员调度台"，而不是每条消息自己起一个。
// 这样切会话时能统一停止上一条朗读，避免多个声音叠加。

/** 浏览器 TTS 引擎实例（全局唯一） */
const synth = typeof window !== 'undefined' ? window.speechSynthesis : null

/** 已加载的语音列表 */
const voices = ref<SpeechSynthesisVoice[]>([])

/** 当前正在朗读的文本（空字符串表示未朗读） */
const speakingText = ref('')

/** 是否暂停 */
const paused = ref(false)

/** 当前朗读的消息 ID（用于 UI 高亮显示） */
const speakingMessageId = ref<string>('')

// ===== 语音加载与选择 =====

/**
 * 加载系统可用的语音列表。
 * 生活类比：开机时先盘点"今天有哪些播音员上班"。
 *
 * 注意：部分浏览器（含 WebView2）的 getVoices() 是异步的，
 * 首次调用可能返回空数组，需要监听 voiceschanged 事件再取一次。
 */
function loadVoices(): void {
  if (!synth) return
  const list = synth.getVoices()
  if (list && list.length > 0) {
    voices.value = list
  }
}

// 启动时立即加载一次 + 监听变化事件
if (synth) {
  loadVoices()
  // onvoiceschanged 在 WebView2 中可靠触发
  if ('onvoiceschanged' in synth) {
    synth.onvoiceschanged = () => loadVoices()
  }
  // 页面卸载/刷新时停止朗读，避免声音残留
  window.addEventListener('beforeunload', () => {
    synth.cancel()
  })
}

/**
 * 中文语音偏好表（按优先级排序）
 * 生活类比：像医院分诊，按病情严重程度排序——
 *   最自然的晓晓排第一，实在没有再用机械感强的兜底声。
 */
const CN_VOICE_PREFERENCES = [
  'Microsoft Xiaoxiao', // Win11 晓晓（最自然女声，首选）
  'Microsoft Yunxi', // Win11 云希（自然男声）
  'Microsoft Yunyang', // Win11 云扬（自然男声）
  'Microsoft Xiaoyi', // Win11 晓伊（自然女声）
  'Microsoft Huihui', // Win10 默认中文女声
  'Microsoft Yaoyao', // Win10 中文女声
  'Microsoft Kangkang' // Win10 中文男声
] as const

/**
 * 从所有语音中挑出最合适的中文语音。
 * 生活类比：人事部按优先级名单挑人——先看首选在不在，不在就依次往下找。
 */
function pickBestChineseVoice(): SpeechSynthesisVoice | null {
  if (voices.value.length === 0) return null

  // 第一步：筛选所有中文语音（lang 以 zh 开头）
  const cnVoices = voices.value.filter(v => v.lang && v.lang.toLowerCase().startsWith('zh'))
  if (cnVoices.length === 0) return null

  // 第二步：按偏好表依次匹配
  for (const preferredName of CN_VOICE_PREFERENCES) {
    const found = cnVoices.find(v => v.name && v.name.includes(preferredName))
    if (found) return found
  }

  // 第三步：偏好表都没匹配上，返回第一个中文语音
  return cnVoices[0]
}

/** 当前选中的语音（响应式，语音列表加载完成后会自动更新） */
const currentVoice = computed<SpeechSynthesisVoice | null>(() => pickBestChineseVoice())

// ===== 用户配置（由外部注入，响应式） =====
// 生活类比：播音员调度台的"设置面板"——用户在设置页调整，这里读取执行。
// 默认值与后端 DefaultConfig 对齐，未注入配置时也能正常工作。

/** 用户配置的发音人名称（空=自动按优先级挑选） */
const configVoiceName = ref<string>('')

/** 用户配置的语速 */
const configRate = ref<number>(1.0)

/** 用户配置的音调 */
const configPitch = ref<number>(1.0)

/** 用户配置的音量 */
const configVolume = ref<number>(1.0)

/**
 * 实际生效的语音（综合用户配置和系统可用语音）
 * 生活类比：用户在设置页点了"晓晓"，就用晓晓；
 *          用户没选（空），就按优先级自动挑。
 */
const effectiveVoice = computed<SpeechSynthesisVoice | null>(() => {
  // 1. 用户明确指定了发音人 → 按名字精确匹配
  const wanted = configVoiceName.value.trim()
  if (wanted) {
    const found = voices.value.find(v => v.name === wanted)
    if (found) return found
    // 找不到指定的发音人 → 退回自动挑选并记录警告
    console.warn(`[TTS] 未找到指定的发音人 "${wanted}"，将自动挑选`)
  }
  // 2. 用户未指定 → 按优先级自动挑选
  return pickBestChineseVoice()
})

/**
 * 更新 TTS 配置（供设置页调用）
 * 生活类比：用户在设置面板调整旋钮，调度台立即记下新设置。
 *
 * @param opts 配置项（全部可选，未传的字段保持原值）
 */
function updateConfig(opts: {
  voice?: string
  rate?: number
  pitch?: number
  volume?: number
}): void {
  if (opts.voice !== undefined) configVoiceName.value = opts.voice
  if (opts.rate !== undefined) configRate.value = opts.rate
  if (opts.pitch !== undefined) configPitch.value = opts.pitch
  if (opts.volume !== undefined) configVolume.value = opts.volume
}

// ===== 朗读控制核心 =====

/**
 * 清理当前朗读状态（内部辅助函数）
 * 生活类比：播音员下班前把话筒、稿子归位。
 */
function resetState(): void {
  speakingText.value = ''
  speakingMessageId.value = ''
  paused.value = false
}

/**
 * 开始朗读文本
 *
 * @param text 要朗读的纯文本（会自动去除 markdown 标记）
 * @param messageId 关联的消息 ID（用于 UI 高亮，可选）
 *
 * 生活类比：把稿子交给播音员，播音员开始念。
 * 如果之前有朗读在进行，会先停掉再开始新的。
 */
function speak(text: string, messageId?: string): void {
  if (!synth) {
    console.warn('[TTS] 浏览器不支持 SpeechSynthesis API')
    return
  }
  if (!text || !text.trim()) return

  // 如果点击的是正在朗读的同一段文本，则停止（切换行为）
  if (speakingText.value === text && speakingMessageId.value === messageId) {
    stop()
    return
  }

  // 停止之前的朗读
  synth.cancel()

  // 清理 markdown 标记，让播音员念纯文字
  // 生活类比：把"**加粗**、#标题、[链接](url)"这些格式符号去掉，
  //          否则播音员会念出"星号星号加粗星号星号"，很难听。
  const cleanText = stripMarkdown(text)

  // 使用浏览器原生构造器创建 utterance
  const utterance = new window.SpeechSynthesisUtterance(cleanText)

  // 配置语音参数（从用户配置读取，默认 1.0/1.0/1.0）
  const voice = effectiveVoice.value
  if (voice) {
    utterance.voice = voice
    utterance.lang = voice.lang
  } else {
    // 没有中文语音时，至少设置 lang 让浏览器尝试
    utterance.lang = 'zh-CN'
  }
  utterance.rate = configRate.value // 语速：用户配置（默认 1.0 = 正常速度）
  utterance.pitch = configPitch.value // 音调：用户配置（默认 1.0 = 正常音调）
  utterance.volume = configVolume.value // 音量：用户配置（默认 1.0 = 最大音量）

  // 朗读事件回调
  utterance.onstart = () => {
    speakingText.value = text
    speakingMessageId.value = messageId || ''
    paused.value = false
  }

  utterance.onend = () => {
    // 朗读正常结束或被 cancel 后都会触发 onend
    // 只有当前确实是这段文本时才重置（避免被新的 utterance 覆盖状态）
    if (speakingText.value === text) {
      resetState()
    }
  }

  utterance.onerror = (event: SpeechSynthesisErrorEvent) => {
    // interrupted（被新的 utterance 打断）是正常行为，不算错误
    if (event.error !== 'interrupted') {
      console.warn('[TTS] 朗读出错:', event.error)
    }
    if (speakingText.value === text) {
      resetState()
    }
  }

  // 启动朗读
  synth.speak(utterance)
}

/**
 * 停止朗读
 * 生活类比：让播音员立刻闭嘴。
 */
function stop(): void {
  if (!synth) return
  synth.cancel()
  resetState()
}

/**
 * 暂停朗读
 * 注意：WebView2 对 pause() 的支持不稳定，部分版本会立即停止而非暂停。
 */
function pause(): void {
  if (!synth) return
  if (synth.speaking && !synth.paused) {
    synth.pause()
    paused.value = true
  }
}

/**
 * 恢复朗读
 */
function resume(): void {
  if (!synth) return
  if (synth.paused) {
    synth.resume()
    paused.value = false
  }
}

// ===== Markdown 清理 =====

/**
 * 去除 Markdown 格式标记，提取纯文本用于朗读。
 * 生活类比：把带格式的"剧本"转成"念稿"——
 *   去掉舞台指示（代码块、图片），只留对白（正文）。
 *
 * 处理规则：
 *   - 代码块 ```...``` → 移除（代码念出来没意义）
 *   - 行内代码 `code` → 移除反引号
 *   - 图片 ![alt](url) → 移除
 *   - 链接 [text](url) → 只保留 text
 *   - 标题/加粗/斜体标记 → 移除符号
 *   - 列表标记 - * 1. → 移除
 *   - 引用 > → 移除
 *   - 水平线 --- → 移除
 *   - 多余空白 → 压缩为单个空格
 */
function stripMarkdown(md: string): string {
  if (!md) return ''

  let text = md

  // 1. 代码块（```...``` 或 ~~~...~~~，跨行）
  text = text.replace(/```[\s\S]*?```/g, ' ')
  text = text.replace(/~~~[\s\S]*?~~~/g, ' ')

  // 2. 行内代码 `code`
  text = text.replace(/`([^`]+)`/g, '$1')

  // 3. 图片 ![alt](url)
  text = text.replace(/!\[([^\]]*)\]\([^)]+\)/g, '$1')

  // 4. 链接 [text](url) → 只保留 text
  text = text.replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')

  // 5. 标题标记 # ## ###
  text = text.replace(/^#{1,6}\s+/gm, '')

  // 6. 加粗/斜体 **text** *text* __text__ _text_
  text = text.replace(/(\*\*|__)(.+?)\1/g, '$2')
  text = text.replace(/(\*|_)(.+?)\1/g, '$2')

  // 7. 列表标记 - * + 1. 2.
  text = text.replace(/^[\s]*[-*+]\s+/gm, '')
  text = text.replace(/^[\s]*\d+\.\s+/gm, '')

  // 8. 引用标记 >
  text = text.replace(/^[\s]*>\s?/gm, '')

  // 9. 水平线 --- ***
  text = text.replace(/^[\s]*([-*]){3,}\s*$/gm, ' ')

  // 10. HTML 标签（如果有）
  text = text.replace(/<[^>]+>/g, ' ')

  // 11. 多余的空白（换行、制表符、多空格）压缩为单个空格
  text = text.replace(/\s+/g, ' ').trim()

  return text
}

// ===== 导出 composable =====

/**
 * TTS composable
 *
 * 用法：
 *   const { speak, stop, isSpeaking, speakingMessageId } = useTTS()
 *
 *   // 朗读消息
 *   speak(message.content, message.id)
 *
 *   // 判断某条消息是否正在朗读
 *   isSpeaking(message.id)
 */
export function useTTS() {
  return {
    // 状态
    voices,
    currentVoice,
    effectiveVoice,
    speakingText,
    speakingMessageId,
    paused,
    isSupported: computed(() => synth !== null),

    // 方法
    speak,
    stop,
    pause,
    resume,
    updateConfig,

    // 工具方法
    /** 判断指定消息是否正在朗读 */
    isSpeaking(messageId: string): boolean {
      return speakingMessageId.value === messageId && speakingText.value !== ''
    },

    /** 判断任意消息是否正在朗读 */
    isAnySpeaking(): boolean {
      return speakingText.value !== ''
    }
  }
}
