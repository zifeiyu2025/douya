<!--
  IconButton: 通用图标按钮
  支持三种 variant: default / primary / danger
  支持 size: small / medium
-->
<template>
    <button
        :class="['icon-btn', `icon-btn-${variant}`, `icon-btn-${size}`, { 'icon-btn-active': active, 'icon-btn-disabled': disabled }]"
        :title="title"
        :disabled="disabled"
        @click="onClick"
    >
        <AppIcon v-if="icon" :name="icon" :size="iconSize" />
        <span v-if="label" class="icon-btn-label">{{ label }}</span>
        <slot />
    </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AppIcon from './AppIcon.vue'

export type IconName =
    | 'audio' | 'video' | 'pdf' | 'file' | 'image'
    | 'send' | 'stop' | 'attach' | 'voice' | 'close' | 'menu'
    | 'search' | 'settings' | 'book' | 'plus' | 'trash' | 'edit'
    | 'theme-sun' | 'theme-moon' | 'back' | 'chevron' | 'mic'
    | 'copy' | 'regenerate' | 'export-md' | 'export-json' | 'export-txt' | 'export-csv'
    | 'minimize' | 'maximize' | 'restore'
    | 'bulb' | 'globe' | 'check' | 'document'

const props = withDefaults(defineProps<{
    icon?: IconName
    label?: string
    title?: string
    variant?: 'default' | 'primary' | 'danger' | 'ghost'
    size?: 'small' | 'medium'
    active?: boolean
    disabled?: boolean
}>(), {
    variant: 'default',
    size: 'medium',
    active: false,
    disabled: false,
})

const emit = defineEmits<{ click: [event: MouseEvent] }>()

const iconSize = computed(() => props.size === 'small' ? 14 : 20)

function onClick(e: MouseEvent) {
    if (props.disabled) return
    emit('click', e)
}
</script>

<style scoped>
.icon-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    border: none;
    background: transparent;
    cursor: pointer;
    color: var(--text-secondary);
    transition: all 0.2s ease;
    font-weight: 500;
    line-height: 1;
    user-select: none;
    white-space: nowrap;
}

.icon-btn-medium {
    padding: 6px 12px;
    border-radius: var(--border-radius-sm);
    font-size: 12.5px;
}

.icon-btn-small {
    padding: 4px 8px;
    border-radius: var(--border-radius-xs);
    font-size: 12px;
}

.icon-btn:hover:not(.icon-btn-disabled) {
    background: var(--bg-hover);
    color: var(--text-primary);
}

.icon-btn-active {
    color: var(--accent-primary);
    background: var(--accent-tertiary);
}

.icon-btn-primary {
    background: var(--accent-primary);
    color: white;
}

.icon-btn-primary:hover:not(.icon-btn-disabled) {
    background: var(--accent-secondary);
    color: white;
}

.icon-btn-danger:hover:not(.icon-btn-disabled) {
    color: var(--accent-danger);
    background: rgba(250, 81, 81, 0.1);
}

.icon-btn-ghost {
    color: var(--text-muted);
}

.icon-btn-disabled {
    opacity: 0.4;
    cursor: not-allowed;
}

.icon-btn-label {
    line-height: 1;
}
</style>
