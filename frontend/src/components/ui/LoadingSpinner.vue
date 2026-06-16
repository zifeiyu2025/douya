<!--
  LoadingSpinner: 统一的 loading spinner
  消除原本散落在多处（App.vue/MessageList.vue/Sidebar.vue）的 @keyframes spin
-->
<template>
    <div
        :class="['loading-spinner', { 'small': size === 'small', 'medium': size === 'medium', 'large': size === 'large' }]"
        :style="spinnerStyle"
    ></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
    size?: 'small' | 'medium' | 'large'
}>(), {
    size: 'small',
})

const SIZE_MAP: Record<string, number> = { small: 16, medium: 24, large: 40 }

const spinnerStyle = computed(() => ({
    width: `${SIZE_MAP[props.size]}px`,
    height: `${SIZE_MAP[props.size]}px`,
    borderWidth: `${props.size === 'large' ? 3 : 2}px`,
}))
</script>

<style scoped>
.loading-spinner {
    border-style: solid;
    border-color: var(--border-color);
    border-top-color: var(--accent-primary);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    box-sizing: border-box;
}

@keyframes spin {
    to { transform: rotate(360deg); }
}
</style>
