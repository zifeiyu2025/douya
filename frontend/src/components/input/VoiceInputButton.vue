<!--
  VoiceInputButton: 语音输入按钮
  从 ChatInput.vue 抽取,封装 SpeechRecognition
-->
<template>
    <button
        :class="['voice-btn', { listening: isListening }]"
        :title="supported ? (isListening ? '停止录音' : '语音输入') : '浏览器不支持'"
        :disabled="!supported"
        @click="onClick"
    >
        <AppIcon :name="isListening ? 'mic' : 'voice'" :size="18" :class="{ pulse: isListening }" />
    </button>
</template>

<script setup lang="ts">
import AppIcon from '../ui/AppIcon.vue'
import { useSpeechRecognition } from '../../composables/useSpeechRecognition'

const props = defineProps<{
    initialText?: string
    disabled?: boolean
}>()

const emit = defineEmits<{
    transcript: [text: string]
}>()

const { isListening, fullText, supported, start, stop } = useSpeechRecognition()

function onClick() {
    if (props.disabled) return
    if (isListening.value) {
        const text = fullText.value
        stop()
        emit('transcript', text)
    } else {
        start(props.initialText || '')
    }
}
</script>

<style scoped>
.voice-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    border-radius: 8px;
    transition: all 0.2s ease;
}

.voice-btn:hover:not(:disabled) {
    background: var(--bg-hover);
    color: var(--text-primary);
}

.voice-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
}

.voice-btn.listening {
    color: var(--accent-danger);
    background: color-mix(in srgb, var(--accent-danger) 12%, transparent);
}

.pulse {
    animation: mic-pulse 1.2s ease-in-out infinite;
}

@keyframes mic-pulse {
    0%, 100% { transform: scale(1); }
    50% { transform: scale(1.15); }
}
</style>
