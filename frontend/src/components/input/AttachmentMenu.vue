<!--
  AttachmentMenu: 附件选择菜单
  从 ChatInput.vue 抽取,包含图片/音频/视频/PDF/文本的入口
-->
<template>
    <div class="attach-wrapper" ref="wrapperRef">
        <button class="attach-btn" @click="toggle" title="添加附件">
            <AppIcon name="attach" :size="18" />
        </button>
        <Transition name="menu-fade">
            <div v-if="show" class="attach-menu glass">
                <button
                    v-for="opt in availableOptions"
                    :key="opt.type"
                    class="menu-item"
                    @click="onPick(opt.type)"
                >
                    <AppIcon :name="opt.icon" :size="16" />
                    <span>{{ opt.label }}</span>
                </button>
            </div>
        </Transition>
        <input
            type="file"
            ref="fileInputRef"
            :accept="currentAccept"
            :multiple="currentType === 'image'"
            style="display:none"
            @change="onFileChange"
        />
    </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import AppIcon from '../ui/AppIcon.vue'
import { getAcceptForType, getTypeLabel } from '../../composables/useFileAttachments'
import type { ModelCapabilities } from '../../types/chat'

const props = defineProps<{
    capabilities: ModelCapabilities
}>()

const emit = defineEmits<{
    files: [files: FileList, type: string]
}>()

const show = ref(false)
const wrapperRef = ref<HTMLElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
const currentType = ref('image')

function toggle() {
    show.value = !show.value
}

const currentAccept = computed(() => getAcceptForType(currentType.value))

const availableOptions = computed(() => {
    const opts: Array<{ type: string; label: string; icon: 'image' | 'audio' | 'video' | 'pdf' | 'file' }> = []
    if (props.capabilities.mmproj_loaded && props.capabilities.image_input) {
        opts.push({ type: 'image', label: '图片', icon: 'image' })
    }
    if (props.capabilities.mmproj_loaded && props.capabilities.audio_input) {
        opts.push({ type: 'audio', label: '音频', icon: 'audio' })
    }
    if (props.capabilities.mmproj_loaded && props.capabilities.video_input) {
        opts.push({ type: 'video', label: '视频', icon: 'video' })
    }
    if (props.capabilities.text_input) {
        opts.push({ type: 'pdf', label: 'PDF', icon: 'pdf' })
        opts.push({ type: 'text', label: '文本', icon: 'file' })
    }
    return opts
})

function onPick(type: string) {
    currentType.value = type
    show.value = false
    nextTick(() => fileInputRef.value?.click())
}

function onFileChange(e: Event) {
    const files = (e.target as HTMLInputElement).files
    if (files && files.length > 0) {
        emit('files', files, currentType.value)
    }
    if (fileInputRef.value) fileInputRef.value.value = ''
}

function onDocClick(e: MouseEvent) {
    const target = e.target as HTMLElement
    if (!target.closest('.attach-wrapper')) {
        show.value = false
    }
}

onMounted(() => {
    document.addEventListener('click', onDocClick)
})

onUnmounted(() => {
    document.removeEventListener('click', onDocClick)
})

void getTypeLabel
</script>

<style scoped>
.attach-wrapper {
    position: relative;
    display: inline-block;
}

.attach-btn {
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

.attach-btn:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
}

.attach-menu {
    position: absolute;
    bottom: calc(100% + 6px);
    left: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 4px;
    border-radius: 10px;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.18);
    min-width: 130px;
    z-index: 100;
}

.menu-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    border: none;
    background: transparent;
    color: var(--text-primary);
    cursor: pointer;
    border-radius: 6px;
    font-size: 13px;
    text-align: left;
}

.menu-item:hover {
    background: var(--bg-hover);
}

.menu-fade-enter-active,
.menu-fade-leave-active {
    transition: opacity 0.15s ease, transform 0.15s ease;
}

.menu-fade-enter-from,
.menu-fade-leave-to {
    opacity: 0;
    transform: translateY(4px);
}
</style>
