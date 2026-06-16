<!--
  SwitchOverlay: 模型切换遮罩
  从 App.vue 抽取,显示切换状态
-->
<template>
    <Transition name="overlay-fade">
        <div v-if="show" class="switch-overlay">
            <div class="switch-card glass">
                <div class="switch-icon" :class="{ failed: hasFailed }">
                    <LoadingSpinner v-if="!hasFailed" size="large" />
                    <AppIcon v-else name="close" :size="32" />
                </div>
                <div class="switch-title">{{ titleText }}</div>
                <div class="switch-model" v-if="modelName">{{ modelName }}</div>
                <div class="switch-detail" v-if="detailText">{{ detailText }}</div>
            </div>
        </div>
    </Transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AppIcon from '../ui/AppIcon.vue'
import LoadingSpinner from '../ui/LoadingSpinner.vue'
import { useSettingsStore } from '../../stores/settings'

const settingsStore = useSettingsStore()

const show = computed(() => settingsStore.isModelSwitching || settingsStore.isFirstLoad || settingsStore.modelLoadFailed)
const hasFailed = computed(() => settingsStore.modelLoadFailed)

const modelName = computed(() => settingsStore.switchingModelDisplay || settingsStore.currentModel)

const titleText = computed(() => {
    if (settingsStore.modelLoadFailed) return '加载失败'
    if (settingsStore.isFirstLoad) return '正在加载模型'
    return '正在切换模型'
})

const detailText = computed(() => {
    if (settingsStore.modelLoadFailed) {
        return settingsStore.switchProgress.errorMessage || '请重试或选择其他模型'
    }
    return '请稍候…'
})
</script>

<style scoped>
.switch-overlay {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 9999;
    background: rgba(0, 0, 0, 0.5);
    backdrop-filter: blur(4px);
}

.switch-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 32px 48px;
    border-radius: 16px;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
    min-width: 280px;
}

.switch-icon {
    color: var(--accent-primary);
}

.switch-icon.failed {
    color: var(--accent-danger);
}

.switch-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-primary);
}

.switch-model {
    font-size: 13px;
    color: var(--text-secondary);
    font-family: var(--font-mono, monospace);
}

.switch-detail {
    font-size: 12px;
    color: var(--text-muted);
}

.overlay-fade-enter-active,
.overlay-fade-leave-active {
    transition: opacity 0.3s ease;
}

.overlay-fade-enter-from,
.overlay-fade-leave-to {
    opacity: 0;
}
</style>
