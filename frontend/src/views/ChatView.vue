<template>
  <div class="chat-container">
    <div class="message-list-wrapper">
      <MessageList />
    </div>
    <ChatInput @send="handleSend" />
    <!-- 采样参数快捷抽屉：开关状态在 useSamplingSettings 模块级单例中，ChatToolbar 直调打开 -->
    <ParamsPanel />
    <!-- AI 内容举报弹窗：模块级单例状态，MessageItem 点"报告问题"触发（商店政策 11.16 合规） -->
    <ReportDialog />
  </div>
</template>

<script setup lang="ts">
import MessageList from '../components/chat/MessageList.vue'
import ChatInput from '../components/chat/ChatInput.vue'
import ParamsPanel from '../components/chat/ParamsPanel.vue'
import ReportDialog from '../components/chat/ReportDialog.vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { useMessage } from 'naive-ui'
import type { Attachment } from '../services/wails'

const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const message = useMessage()

async function handleSend(content: string, images?: string[], attachments?: Attachment[]) {
  // 安全网：搜索模式开启但无 API Key 时阻止发送并提示
  if (settingsStore.searchMode !== 'off') {
    await settingsStore.loadSearchAPIKeys()
    const keys = settingsStore.searchAPIKeys
    if (!keys.tavily_api_key_set && !keys.ollama_api_key_set) {
      message.warning(
        '未配置搜索 API Key，联网搜索无法生效。请在「设置 → 联网搜索」中配置 API Key 后再试',
        { duration: 5000 }
      )
      return
    }
  }
  chatStore.sendMessage(content, settingsStore.searchMode, images, attachments)
}
</script>

<style scoped>
/* 聊天主容器：纵向弹性列，撑满 .main-area 高度——
   消息列表（flex:1）与输入框（固定高）的布局基石 */
.chat-container {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.message-list-wrapper {
  flex: 1;
  min-height: 0;
  height: 0;
  overflow: hidden;
}
</style>
