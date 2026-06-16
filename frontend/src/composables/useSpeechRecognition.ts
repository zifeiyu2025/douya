/**
 * 语音识别 composable
 * 封装 SpeechRecognition API
 */
import { ref, computed, onUnmounted } from 'vue'

export interface SpeechRecognitionEvent {
    results: SpeechRecognitionResultList
    resultIndex: number
}

export interface SpeechRecognitionErrorEvent {
    error: string
    message: string
}

export interface SpeechRecognitionInstance {
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

export function isSpeechRecognitionSupported(): boolean {
    return !!(typeof window !== 'undefined' && (window.SpeechRecognition || window.webkitSpeechRecognition))
}

export function useSpeechRecognition() {
    const isListening = ref(false)
    const interimText = ref('')
    const supported = computed(() => isSpeechRecognitionSupported())
    let recognition: SpeechRecognitionInstance | null = null
    let finalBuffer = ''

    function init() {
        const Ctor = window.SpeechRecognition || window.webkitSpeechRecognition
        if (!Ctor) return

        recognition = new Ctor()
        recognition.lang = 'zh-CN'
        recognition.continuous = true
        recognition.interimResults = true
        recognition.maxAlternatives = 1

        recognition.onresult = (event) => {
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
                finalBuffer += finalTranscript
            }
            interimText.value = interim
        }

        recognition.onerror = (event) => {
            console.warn('语音识别错误:', event.error)
            if (event.error === 'not-allowed') {
                isListening.value = false
                interimText.value = ''
            }
        }

        recognition.onend = () => {
            if (isListening.value) {
                try {
                    recognition?.start()
                } catch {
                    isListening.value = false
                    interimText.value = ''
                }
            } else {
                interimText.value = ''
            }
        }

        recognition.onstart = () => {
            isListening.value = true
        }
    }

    function start(initialText = '') {
        if (!recognition) {
            init()
        }
        if (!recognition) return

        finalBuffer = initialText
        interimText.value = ''

        try {
            recognition.start()
        } catch {
            recognition = null
            init()
            if (recognition !== null) {
                (recognition as SpeechRecognitionInstance).start()
            }
        }
    }

    function stop() {
        if (recognition) {
            isListening.value = false
            recognition.stop()
        }
        interimText.value = ''
        finalBuffer = ''
    }

    function abort() {
        if (recognition) {
            isListening.value = false
            recognition.abort()
            recognition = null
        }
    }

    /** 完整文本 = 已确认的 final buffer + 临时识别结果 */
    const fullText = computed(() => {
        return finalBuffer + interimText.value
    })

    onUnmounted(() => {
        abort()
    })

    return {
        isListening,
        interimText,
        fullText,
        supported,
        start,
        stop,
        abort,
    }
}
