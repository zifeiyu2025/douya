/**
 * useTTS.ts — 文本转语音（TTS）composable
 *
 * 生活类比：这个文件就像豆芽的"播音员调度员"——
 *   - 它知道系统里有哪些"本地播音员"（voices，Web Speech API）
 *   - 它还会在有网时请"微软在线播音员"（Edge TTS / 晓晓）帮忙
 *   - 它负责安排播音员什么时候开始、暂停、停止
 *
 * 朗读策略（按用户需求）：
 *   - 开启「在线 TTS」且有网：优先调用微软在线 TTS（Edge TTS），
 *     按设置页选的本地发音人映射到对应在线 Neural 音色（如晓晓→XiaoxiaoNeural），未选则用晓晓
 *   - 无网 / 在线失败 / 关闭在线：自动回退到浏览器本地 Web Speech API（Web Speech API）
 *
 * 本地语音基于 W3C Web Speech API 的 SpeechSynthesis 接口（WebView2 完整支持）。
 * 在线语音基于 Edge TTS（直连微软云端，免 API Key）。
 *
 * 中文本地语音优先级（从高到低）：
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
import { logWarn } from '../utils/logger'
import { wails as ttsService } from '../services/wails'

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

/**
 * 当前实际使用的后端：'online'（微软在线晓晓）/ 'local'（本地 Web Speech） / ''（未朗读）
 * 用于 UI 显示「在线/本地」状态徽标。
 */
const currentBackend = ref<'' | 'online' | 'local'>('')

/** 在线播放时的 HTMLAudioElement 实例（本地用 speechSynthesis，不在此列） */
let currentAudio: HTMLAudioElement | null = null

/**
 * 本地逐句朗读的取消令牌。
 * 每次开始新朗读 / 显式停止都自增，进行中的"逐句链条"检测到令牌不符即终止。
 * 这是修复 Chromium/WebView2「长文本被静默中断后从头重播（表现为一直循环播放）」的关键：
 * 停止时自增令牌，cancel 触发的 onend 不会再链式播下一句。
 */
let localSpeechToken = 0

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

/** 用户配置的发音人名称（空=自动按优先级挑选），仅本地回退时使用 */
const configVoiceName = ref<string>('')

/** 用户配置的语速 */
const configRate = ref<number>(1.0)

/** 用户配置的音调 */
const configPitch = ref<number>(1.0)

/** 用户配置的音量 */
const configVolume = ref<number>(1.0)

/** 是否优先使用在线 TTS（微软晓晓）；关闭或离线时回退本地 */
const configOnline = ref<boolean>(true)

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
    logWarn(`[TTS] 未找到指定的发音人 "${wanted}"，将自动挑选`)
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
  online?: boolean
}): void {
  if (opts.voice !== undefined) configVoiceName.value = opts.voice
  if (opts.rate !== undefined) configRate.value = opts.rate
  if (opts.pitch !== undefined) configPitch.value = opts.pitch
  if (opts.volume !== undefined) configVolume.value = opts.volume
  if (opts.online !== undefined) configOnline.value = opts.online
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
  currentBackend.value = ''
}

/** 停止进行中的朗读（本地 + 在线），不重置外部标记 */
function stopAllInternal(): void {
  localSpeechToken++ // 使任何进行中的本地逐句链条失效，避免 cancel 后继续播下一句
  if (synth) synth.cancel()
  if (currentAudio) {
    try {
      currentAudio.pause()
    } catch {
      /* 忽略暂停异常 */
    }
    currentAudio = null
  }
}

/**
 * 将 base64 字符串解码为二进制字节（用于在线 MP3 播放）。
 */
function base64ToBytes(b64: string): Uint8Array<ArrayBuffer> {
  const bin = atob(b64)
  const len = bin.length
  const bytes = new Uint8Array(new ArrayBuffer(len))
  for (let i = 0; i < len; i++) {
    bytes[i] = bin.charCodeAt(i)
  }
  return bytes
}

/**
 * 给 Promise 加超时，避免在线合成卡死时前端一直等待。
 */
function withTimeout<T>(p: Promise<T>, ms: number): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('online TTS timeout')), ms)
    p.then(
      v => {
        clearTimeout(timer)
        resolve(v)
      },
      e => {
        clearTimeout(timer)
        reject(e)
      }
    )
  })
}

/**
 * 把长文本按句子切分，避免单个超长 utterance 在 Chromium/WebView2 中
 * 约 15 秒处被静默中断并从头重播（表现为"一直循环播放"）。
 * 优先按句末标点切句（最自然），超长无标点段落再按字数硬切。
 */
function splitIntoChunks(text: string): string[] {
  const segs = text
    .replace(/\r\n/g, '\n')
    .split(/(?<=[。！？!?；;\n])/)
    .map(s => s.trim())
    .filter(s => s.length > 0)

  const chunks: string[] = []
  const MAX = 100 // 单句上限（约 15~20 秒），超限再硬切以防被引擎截断重播
  for (const seg of segs) {
    if (seg.length <= MAX) {
      chunks.push(seg)
      continue
    }
    for (let i = 0; i < seg.length; i += MAX) {
      const piece = seg.slice(i, i + MAX).trim()
      if (piece) chunks.push(piece)
    }
  }
  return chunks.length > 0 ? chunks : [text]
}

/**
 * 本地播放（浏览器 Web Speech API）——逐句顺序朗读，播完自动停止。
 * 生活类比：请本地的播音员念稿，一段一段念，全部念完就下班，不会从头重播。
 *
 * 关键点：用 localSpeechToken 串联各句；某句 onend 时若令牌已被新朗读/停止取代，
 * 则不再播下一句，从而彻底避免"循环播放"。
 */
function playLocal(cleanText: string, text: string, messageId?: string): void {
  if (!synth) {
    logWarn('[TTS] 浏览器不支持 SpeechSynthesis API，无法本地朗读')
    return
  }

  const chunks = splitIntoChunks(cleanText)
  let idx = 0
  const token = ++localSpeechToken // 本次朗读的令牌
  currentBackend.value = 'local'

  const speakNext = (): void => {
    if (token !== localSpeechToken) return // 已被新朗读 / 停止取代，终止链条
    if (idx >= chunks.length) {
      if (speakingText.value === text) resetState()
      return
    }
    const chunk = chunks[idx++]
    if (!chunk) {
      speakNext()
      return
    }

    const utterance = new window.SpeechSynthesisUtterance(chunk)
    const voice = effectiveVoice.value
    if (voice) {
      utterance.voice = voice
      utterance.lang = voice.lang
    } else {
      utterance.lang = 'zh-CN'
    }
    utterance.rate = configRate.value
    utterance.pitch = configPitch.value
    utterance.volume = configVolume.value

    utterance.onstart = () => {
      if (token !== localSpeechToken) return
      speakingText.value = text
      speakingMessageId.value = messageId || ''
      paused.value = false
      currentBackend.value = 'local'
    }
    utterance.onend = () => {
      if (token !== localSpeechToken) return
      speakNext() // 播下一句；全部结束则复位状态
    }
    utterance.onerror = (event: SpeechSynthesisErrorEvent) => {
      if (token !== localSpeechToken) return
      if (event.error !== 'interrupted') {
        logWarn('[TTS] 本地朗读出错', event.error)
      }
      speakNext() // 出错也继续下一句（被取消已由令牌拦截）；全部结束则复位
    }

    synth.speak(utterance)
  }

  speakNext()
}

/**
 * 在线播放（微软 Edge TTS 返回的 MP3，用 <audio> 元素播放）
 * 生活类比：把云端播音员录好的音频放出来。播放失败则回退本地播音员。
 */
function playOnlineAudio(b64: string, text: string, messageId?: string): void {
  try {
    const bytes = base64ToBytes(b64)
    const blob = new Blob([bytes], { type: 'audio/mpeg' })
    const url = URL.createObjectURL(blob)
    const audio = new Audio(url)
    currentAudio = audio
    currentBackend.value = 'online'

    const cleanup = () => {
      URL.revokeObjectURL(url)
      if (currentAudio === audio) currentAudio = null
    }

    audio.onplay = () => {
      speakingText.value = text
      speakingMessageId.value = messageId || ''
      paused.value = false
      currentBackend.value = 'online'
    }
    audio.onended = () => {
      if (speakingText.value === text) resetState()
      cleanup()
    }
    audio.onerror = () => {
      cleanup()
      // 在线音频播放失败，回退本地播音员
      if (speakingText.value === text) {
        playLocal(stripMarkdown(text), text, messageId)
      }
    }

    audio.play().catch(() => {
      cleanup()
      if (speakingText.value === text) {
        playLocal(stripMarkdown(text), text, messageId)
      }
    })
  } catch (e) {
    logWarn('[TTS] 在线音频播放失败，回退本地', (e as Error)?.message)
    const clean = stripMarkdown(text)
    if (speakingText.value === text) playLocal(clean, text, messageId)
  }
}

/**
 * 开始朗读文本
 *
 * @param text 要朗读的纯文本（会自动去除 markdown 标记）
 * @param messageId 关联的消息 ID（用于 UI 高亮，可选）
 *
 * 生活类比：把稿子交给调度员，调度员先问"在线播音员（晓晓）在不在"——
 *   在（有网）就请她念；不在（无网）就请本地播音员念。
 * 如果之前有朗读在进行，会先停掉再开始新的。
 */
async function speak(text: string, messageId?: string): Promise<void> {
  if (!text || !text.trim()) return

  // 如果点击的是正在朗读的同一段文本，则停止（切换行为）
  if (speakingText.value === text && speakingMessageId.value === messageId) {
    stop()
    return
  }

  // 停止之前的朗读（本地 + 在线）
  stopAllInternal()

  const cleanText = stripMarkdown(text)
  if (!cleanText) return

  // 立即标记，供 UI 高亮 + 切换检测（无论在线/本地，先占住这个朗读槽）
  speakingText.value = text
  speakingMessageId.value = messageId || ''
  paused.value = false
  currentBackend.value = ''

  if (configOnline.value) {
    try {
      // 调用后端 Edge TTS 合成（后端内部会做网络探测，无网快速失败）；
      // 前端再套一层超时，避免后端卡死时一直等待。
      // 传入设置页选的发音人名：本地发音人（如 "Microsoft Xiaoxiao"）会由后端映射为对应在线
      // Neural 音色；仅在线发音人则直接传 Neural 名（如 "zh-CN-XiaochenNeural"），后端原样透传；
      // 未选（空）时后端默认使用微软晓晓。本地回退仍用 effectiveVoice（自动挑选）。
      // 注意：第一个参数是要朗读的纯文本（cleanText），绝不能误传成发音人名，
      // 否则后端收到空/错误文本导致在线合成失败，回退本地后长文本首句会触发 Chromium
      // 的 15 秒截断重播缺陷（表现为"重复全文中第一个句子"）。
      const b64 = await withTimeout(
        ttsService.synthesizeSpeech(
          cleanText,
          configVoiceName.value.trim(),
          configRate.value,
          configPitch.value,
          configVolume.value
        ),
        9000
      )
      // 合成期间若已被用户停止（点击同一段/切换），speakingText 已清空，则放弃播放
      if (speakingText.value !== text) return
      playOnlineAudio(b64, text, messageId)
      return
    } catch (e) {
      logWarn('[TTS] 在线合成失败，回退本地', (e as Error)?.message)
      // 回退本地
    }
  }

  // 离线回退 / 在线关闭 / 在线失败：用本地播音员
  if (speakingText.value === text) {
    playLocal(cleanText, text, messageId)
  }
}

/**
 * 停止朗读
 * 生活类比：让播音员立刻闭嘴。
 */
function stop(): void {
  stopAllInternal()
  resetState()
}

/**
 * 暂停朗读
 * 注意：WebView2 对 pause() 的支持不稳定，部分版本会立即停止而非暂停。
 */
function pause(): void {
  if (currentBackend.value === 'online' && currentAudio) {
    currentAudio.pause()
    paused.value = true
    return
  }
  if (synth && synth.speaking && !synth.paused) {
    synth.pause()
    paused.value = true
  }
}

/**
 * 恢复朗读
 */
function resume(): void {
  if (currentBackend.value === 'online' && currentAudio) {
    currentAudio.play().catch(() => {
      /* 忽略播放异常 */
    })
    paused.value = false
    return
  }
  if (synth && synth.paused) {
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
    currentBackend,
    configOnline,
    // 在线可用（本地或在线任一可用即可显示朗读按钮）
    isSupported: computed(() => synth !== null || configOnline.value),

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
