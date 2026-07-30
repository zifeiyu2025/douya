<template>
  <div class="chat-container">
    <div class="message-list-wrapper">
      <MessageList />
    </div>
    <ChatInput @send="handleSend" />
  </div>
</template>

<script setup lang="ts">
import MessageList from '../components/MessageList.vue'
import ChatInput from '../components/ChatInput.vue'
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
.message-list-wrapper {
  flex: 1;
  min-height: 0;
  height: 0;
  overflow: hidden;
}
</style>
