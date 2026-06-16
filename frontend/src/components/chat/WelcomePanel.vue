<!--
  WelcomePanel: 空对话时的欢迎页
  从 MessageList.vue 抽取,展示快捷提示和最近使用
-->
<template>
    <div class="welcome-panel">
        <div class="welcome-icon glass-icon">
            <img :src="appiconUrl" alt="豆芽" class="welcome-logo-img" />
        </div>
        <div class="welcome-title">开始一个新对话</div>
        <div class="welcome-subtitle">支持联网搜索、多模态输入、长对话记忆</div>
        <div class="quick-tips">
            <div class="tip-item" v-for="(tip, i) in tips" :key="i">
                <AppIcon :name="tip.icon" :size="14" />
                <span>{{ tip.text }}</span>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import AppIcon from '../ui/AppIcon.vue'
import appiconUrl from '../../assets/images/appicon.png'

const tips = [
    { icon: 'globe' as const, text: '支持联网搜索,回答包含最新信息' },
    { icon: 'image' as const, text: '支持图片/音频/视频多模态' },
    { icon: 'document' as const, text: '支持 PDF/文本附件解析' },
]
</script>

<style scoped>
.welcome-panel {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 16px;
    padding: 60px 24px;
    text-align: center;
}

.welcome-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 64px;
    height: 64px;
    border-radius: 50%;
    background: var(--bg-tertiary);
    color: var(--accent-primary);
}

.welcome-logo-img {
    width: 40px;
    height: 40px;
    border-radius: 10px;
    object-fit: contain;
}

.glass-icon {
    background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
    backdrop-filter: blur(8px);
}

.welcome-title {
    font-size: 20px;
    font-weight: 600;
    color: var(--text-primary);
}

.welcome-subtitle {
    font-size: 13px;
    color: var(--text-secondary);
}

.quick-tips {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-top: 12px;
}

.tip-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 16px;
    border-radius: 8px;
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    font-size: 12.5px;
}
</style>
