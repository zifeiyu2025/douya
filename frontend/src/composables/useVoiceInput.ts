/**
 * 语音输入 composable
 *
 * 生活类比：就像一个语音速记员——你说话，它把声音变成文字填进输入框。
 * 说话过程中会实时显示"听到的"临时文字，停顿后把确认的文字追加到输入框。
 *
 * 从 ChatInput.vue 抽取（基于架构优化：ChatInput.vue 1789 行→拆分独立职责）：
 * - SpeechRecognition 类型定义
 * - 语音识别状态管理（isListening, voiceInterimText）
 * - 语音识别生命周期（init/start/stop/cleanup）
 */
import { ref, computed, type Ref } from 'vue'

// ===== SpeechRecognition 类型定义 =====
// 浏览器原生 API 没有完整的 TS 类型，这里手动声明用到的部分

interface SpeechRecognitionEvent {
  results: SpeechRecognitionResultList
  resultIndex: number
}

interface SpeechRecognitionErrorEvent {
  error: string
  message: string
}

interface SpeechRecognitionInstance {
  lang: string
  continuous: boolean
  interimResults: boolean
  maxAlternatives: number
  onresult: ((event: SpeechRecognitionEvent) => void) | null
  onerror: ((event: SpeechRecognitionErrorEvent) => void) | null
  onend: (() => void) | null
  onstart: (() => void) | null
  start: () => void
  stop: () => void
  abort: () => void
}

declare global {
  interface Window {
    SpeechRecognition?: new () => SpeechRecognitionInstance
    webkitSpeechRecognition?: new () => SpeechRecognitionInstance
  }
}

/**
 * @param inputText 输入框文本（双向：语音识别结果会写入此 ref）
 */
export function useVoiceInput(inputText: Ref<string>) {
  const isListening = ref(false)
  const voiceInterimText = ref('')
  let recognition: SpeechRecognitionInstance | null = null
  let voiceFinalBuffer = ''

  const speechSupported = computed(() => {
    return !!(window.SpeechRecognition || window.webkitSpeechRecognition)
  })

  function initSpeechRecognition() {
    const SpeechRecognitionCtor = window.SpeechRecognition || window.webkitSpeechRecognition
    if (!SpeechRecognitionCtor) return

    recognition = new SpeechRecognitionCtor()
    recognition.lang = 'zh-CN'
    recognition.continuous = true
    recognition.interimResults = true
    recognition.maxAlternatives = 1

    recognition.onresult = (event: SpeechRecognitionEvent) => {
      let interim = ''
      let finalTranscript = ''
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const result = event.results[i]
        if (result.isFinal) {
          finalTranscript += result[0].transcript
        } else {
          interim += result[0].transcript
        }
      }
      if (finalTranscript) {
        voiceFinalBuffer += finalTranscript
        inputText.value = voiceFinalBuffer + interim
      } else {
        inputText.value = voiceFinalBuffer + interim
      }
      voiceInterimText.value = interim
    }

    recognition.onerror = (event: SpeechRecognitionErrorEvent) => {
      console.warn('语音识别错误:', event.error)
      if (event.error === 'not-allowed') {
        isListening.value = false
        voiceInterimText.value = ''
      }
    }

    recognition.onend = () => {
      if (isListening.value) {
        try {
          recognition?.start()
        } catch {
          isListening.value = false
          voiceInterimText.value = ''
        }
      } else {
        voiceInterimText.value = ''
      }
    }

    recognition.onstart = () => {
      isListening.value = true
    }
  }

  function toggleListening() {
    if (!speechSupported.value) return
    if (isListening.value) {
      stopListening()
    } else {
      startListening()
    }
  }

  function doStartRecognition(rec: SpeechRecognitionInstance) {
    try {
      rec.start()
    } catch {
      console.warn('无法启动语音识别')
    }
  }

  function startListening() {
    if (!recognition) {
      initSpeechRecognition()
    }
    if (!recognition) return

    voiceFinalBuffer = inputText.value
    voiceInterimText.value = ''

    try {
      recognition.start()
    } catch {
      recognition = null
      initSpeechRecognition()
      if (recognition) {
        doStartRecognition(recognition)
      }
    }
  }

  function stopListening() {
    if (recognition) {
      isListening.value = false
      recognition.stop()
    }
    voiceInterimText.value = ''
    voiceFinalBuffer = ''
  }

  /** 组件卸载时调用，释放语音识别资源 */
  function cleanup() {
    if (recognition) {
      isListening.value = false
      recognition.abort()
      recognition = null
    }
  }

  return {
    isListening,
    voiceInterimText,
    speechSupported,
    initSpeechRecognition,
    toggleListening,
    stopListening,
    cleanup
  }
}
