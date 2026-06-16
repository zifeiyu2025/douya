<!--
  TitleBar: 顶部窗口栏
  包含窗口拖拽区、模型选择器、状态徽章、主题切换、窗口控制按钮
-->
<template>
    <div class="title-bar" style="--wails-draggable:drag">
        <div class="title-bar-left">
            <slot name="left" />
        </div>
        <div class="title-bar-center">
            <slot name="center" />
        </div>
        <div class="title-bar-right">
            <slot name="right" />
            <button class="window-btn" @click="minimize" title="最小化">
                <AppIcon name="minimize" :size="14" />
            </button>
            <button class="window-btn" @click="toggleMaximize" title="最大化">
                <AppIcon :name="isMaximized ? 'restore' : 'maximize'" :size="12" />
            </button>
            <button class="window-btn close-btn" @click="close" title="关闭">
                <AppIcon name="close" :size="14" />
            </button>
        </div>
    </div>
</template>

<script setup lang="ts">
import AppIcon from '../ui/AppIcon.vue'
import { useWindowControls } from '../../composables/useWindowControls'

const { isMaximized, minimize, toggleMaximize, close } = useWindowControls()
</script>

<style scoped>
.title-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 38px;
    padding: 0 12px;
    background: var(--bg-secondary);
    border-bottom: 1px solid var(--border-color);
    flex-shrink: 0;
}

.title-bar-left,
.title-bar-center,
.title-bar-right {
    display: flex;
    align-items: center;
    gap: 8px;
}

.title-bar-right {
    -webkit-app-region: no-drag;
}

.title-bar-center {
    flex: 1;
    justify-content: center;
}

.window-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border: none;
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    border-radius: 6px;
    transition: all 0.15s ease;
}

.window-btn:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
}

.close-btn:hover {
    background: var(--accent-danger);
    color: white;
}
</style>
