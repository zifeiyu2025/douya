<!--
  ExitOverlay: 退出确认遮罩
-->
<template>
    <Teleport to="body">
        <Transition name="exit-fade">
            <div v-if="show" class="exit-overlay">
                <div class="exit-card glass">
                    <div class="exit-title">确认退出？</div>
                    <div class="exit-message">退出前会保存当前会话,并确保模型服务正确关闭。</div>
                    <div class="exit-actions">
                        <button class="exit-btn cancel" @click="onCancel">取消</button>
                        <button class="exit-btn confirm" @click="onConfirm">确认退出</button>
                    </div>
                </div>
            </div>
        </Transition>
    </Teleport>
</template>

<script setup lang="ts">
defineProps<{ show: boolean }>()
const emit = defineEmits<{
    confirm: []
    cancel: []
}>()

function onConfirm() { emit('confirm') }
function onCancel() { emit('cancel') }
</script>

<style scoped>
.exit-overlay {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 10000;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(6px);
}

.exit-card {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 24px 28px;
    border-radius: 14px;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.35);
    min-width: 320px;
}

.exit-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-primary);
}

.exit-message {
    font-size: 13px;
    color: var(--text-secondary);
    line-height: 1.5;
}

.exit-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    margin-top: 8px;
}

.exit-btn {
    padding: 6px 16px;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-size: 13px;
    transition: all 0.2s ease;
}

.exit-btn.cancel {
    background: var(--bg-tertiary);
    color: var(--text-primary);
}

.exit-btn.cancel:hover {
    background: var(--bg-hover);
}

.exit-btn.confirm {
    background: var(--accent-danger);
    color: white;
}

.exit-btn.confirm:hover {
    filter: brightness(0.9);
}

.exit-fade-enter-active,
.exit-fade-leave-active {
    transition: opacity 0.25s ease;
}

.exit-fade-enter-from,
.exit-fade-leave-to {
    opacity: 0;
}
</style>
