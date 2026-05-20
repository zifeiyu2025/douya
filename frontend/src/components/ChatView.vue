<template>
  <div class="chat-container">
    <div class="message-list-wrapper">
      <MessageList />
    </div>
    <ChatInput @send="handleSend" />
  </div>
</template>

<script setup lang="ts">
import MessageList from './MessageList.vue'
import ChatInput from './ChatInput.vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import type { Attachment } from '../services/wails'

const chatStore = useChatStore()
const settingsStore = useSettingsStore()

function handleSend(content: string, images?: string[], attachments?: Attachment[]) {
  chatStore.sendMessage(content, settingsStore.searchEnabled, images, attachments)
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
