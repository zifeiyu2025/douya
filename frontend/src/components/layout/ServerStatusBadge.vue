<!--
  ServerStatusBadge: 服务器状态徽章
  从 App.vue 抽取,显示 server:running/model_ready/error
-->
<template>
    <div :class="['server-badge', `status-${statusLevel}`]" @click="onClick">
        <span class="dot" />
        <span class="text">{{ statusText }}</span>
    </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useSettingsStore } from '../../stores/settings'

const settingsStore = useSettingsStore()

const statusLevel = computed<'ready' | 'loading' | 'error' | 'off'>(() => {
    const s = settingsStore.serverStatus
    if (s.error) return 'error'
    if (s.model_ready) return 'ready'
    if (s.running) return 'loading'
    return 'off'
})

const statusText = computed(() => {
    const s = settingsStore.serverStatus
    if (s.error) return '错误'
    if (s.model_ready) return '就绪'
    if (s.running) return '加载中'
    return '离线'
})

function onClick() {
    // 触发 status 重新拉取
    settingsStore.checkServerStatus()
}
</script>

<style scoped>
.server-badge {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 14px;
    background: var(--bg-tertiary);
    cursor: pointer;
    font-size: 12px;
    user-select: none;
    transition: background 0.2s ease;
}

.server-badge:hover {
    background: var(--bg-hover);
}

.dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--text-muted);
}

.status-ready .dot {
    background: var(--accent-success, #4ade80);
    box-shadow: 0 0 6px var(--accent-success, #4ade80);
}

.status-loading .dot {
    background: var(--accent-warning, #fbbf24);
    animation: pulse 1.4s ease-in-out infinite;
}

.status-error .dot {
    background: var(--accent-danger, #f87171);
}

.text {
    color: var(--text-secondary);
}

@keyframes pulse {
    0%, 100% { opacity: 0.4; }
    50% { opacity: 1; }
}
</style>
