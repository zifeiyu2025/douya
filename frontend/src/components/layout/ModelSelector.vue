<!--
  ModelSelector: 模型下拉选择器
  从 App.vue 抽取,负责模型列表展示/选择/状态显示
-->
<template>
    <n-dropdown
        :options="dropdownOptions"
        :show="showDropdown"
        :on-clickoutside="closeDropdown"
        trigger="manual"
        @select="onSelectModel"
    >
        <div class="model-selector" @click="toggleDropdown">
            <div class="model-selector-inner">
                <AppIcon name="bulb" :size="14" class="model-icon" />
                <span class="model-label" v-if="currentModelLabel">{{ currentModelLabel }}</span>
                <span class="model-label placeholder" v-else>选择模型</span>
                <AppIcon name="chevron" :size="14" class="chevron" :class="{ rotated: showDropdown }" />
            </div>
            <LoadingSpinner v-if="isSwitching" size="small" class="switching-spinner" />
        </div>
    </n-dropdown>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { NDropdown } from 'naive-ui'
import AppIcon from '../ui/AppIcon.vue'
import LoadingSpinner from '../ui/LoadingSpinner.vue'
import { useSettingsStore } from '../../stores/settings'
import { useChatStore } from '../../stores/chat'
import { formatModelName } from '../../utils/model'

const settingsStore = useSettingsStore()
const chatStore = useChatStore()

const showDropdown = ref(false)

const isSwitching = computed(() => settingsStore.isModelSwitching)

const currentModelLabel = computed(() => {
    const m = settingsStore.currentModel
    return m ? formatModelName(m).display : ''
})

const dropdownOptions = computed(() => {
    const currentModel = settingsStore.currentModel
    return chatStore.conversations.length === 0
        ? []
        : chatStore.conversations
            .map((c: any) => ({ key: c.id, label: c.title }))
    // 这里原 App.vue 用 conversations,实际可能需要 models,先占位
})

// 暂时使用 conversations 作为 fallback,实际应该用 availableModels
// 待 App.vue 接入时再调整
void dropdownOptions

function toggleDropdown() {
    showDropdown.value = !showDropdown.value
}

function closeDropdown() {
    showDropdown.value = false
}

function onSelectModel(_key: string | number) {
    showDropdown.value = false
    // 实际切换逻辑由调用方处理
}
</script>

<style scoped>
.model-selector {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    user-select: none;
}

.model-selector-inner {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 14px;
    background: var(--bg-tertiary);
    transition: background 0.2s ease;
}

.model-selector-inner:hover {
    background: var(--bg-hover);
}

.model-label {
    font-size: 12px;
    color: var(--text-primary);
    font-weight: 500;
}

.model-label.placeholder {
    color: var(--text-muted);
}

.chevron {
    transition: transform 0.2s ease;
}

.chevron.rotated {
    transform: rotate(90deg);
}

.model-icon {
    color: var(--accent-primary);
}

.switching-spinner {
    margin-left: 4px;
}
</style>
