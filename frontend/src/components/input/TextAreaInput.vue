<!--
  TextAreaInput: 多行文本输入框,自动高度
  从 ChatInput.vue 抽取
-->
<template>
    <textarea
        ref="textareaRef"
        :value="modelValue"
        :placeholder="placeholder"
        :rows="1"
        :disabled="disabled"
        class="text-area-input"
        @input="onInput"
        @keydown="onKeydown"
        @paste="onPaste"
    />
</template>

<script setup lang="ts">
import { ref, watch, nextTick, onMounted } from 'vue'

const props = withDefaults(defineProps<{
    modelValue: string
    placeholder?: string
    disabled?: boolean
    minRows?: number
    maxRows?: number
    enableSendShortcut?: boolean
}>(), {
    placeholder: '输入消息…',
    disabled: false,
    minRows: 1,
    maxRows: 8,
    enableSendShortcut: true,
})

const emit = defineEmits<{
    'update:modelValue': [value: string]
    send: []
}>()

const textareaRef = ref<HTMLTextAreaElement | null>(null)

function autoHeight() {
    const el = textareaRef.value
    if (!el) return
    el.style.height = 'auto'
    const lineHeight = 22
    const maxH = lineHeight * props.maxRows
    const newH = Math.min(el.scrollHeight, maxH)
    el.style.height = `${newH}px`
}

function onInput(e: Event) {
    const val = (e.target as HTMLTextAreaElement).value
    emit('update:modelValue', val)
    nextTick(autoHeight)
}

function onKeydown(e: KeyboardEvent) {
    if (props.enableSendShortcut && e.key === 'Enter' && !e.shiftKey && !e.ctrlKey && !e.altKey) {
        e.preventDefault()
        emit('send')
    }
}

function onPaste(e: ClipboardEvent) {
    // 占位:由调用方处理附件粘贴
    void e
}

watch(() => props.modelValue, () => {
    nextTick(autoHeight)
})

onMounted(() => {
    autoHeight()
})

defineExpose({
    focus: () => textareaRef.value?.focus(),
    blur: () => textareaRef.value?.blur(),
})
</script>

<style scoped>
.text-area-input {
    width: 100%;
    min-height: 22px;
    max-height: 200px;
    padding: 4px 0;
    background: transparent;
    border: none;
    color: var(--text-primary);
    font-family: inherit;
    font-size: 14px;
    line-height: 22px;
    resize: none;
    outline: none;
    overflow-y: auto;
}

.text-area-input::placeholder {
    color: var(--text-muted);
}

.text-area-input:disabled {
    opacity: 0.5;
    cursor: not-allowed;
}
</style>
