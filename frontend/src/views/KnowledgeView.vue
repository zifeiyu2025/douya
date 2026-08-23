<!--
  KnowledgeView: 知识库管理视图（C-6 编排壳）
  三段子组件：KBSelector（知识库）/ DocumentManager（文档管理）/ RagSettings（检索设置）。
  状态与逻辑集中在 useKnowledge 单一状态源，经 provide 注入子组件。
-->
<template>
  <div class="knowledge-container">
    <div class="knowledge-header">
      <button class="back-btn" type="button" aria-label="返回" @click="$router.push('/')">
        <svg width="20" height="20" viewBox="0 0 512 512" fill="none" aria-hidden="true">
          <path
            d="M244 400L100 256l144-144M120 256h292"
            stroke="currentColor"
            stroke-width="48"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </button>
      <div class="header-title-group">
        <span class="knowledge-title">知识库管理</span>
        <span class="knowledge-subtitle">管理文档和检索设置</span>
      </div>
    </div>

    <div class="knowledge-body">
      <KBSelector />
      <DocumentManager />
      <RagSettings />
    </div>
  </div>
</template>

<script setup lang="ts">
import { provide, onMounted } from 'vue'
import KBSelector from '../components/knowledge/KBSelector.vue'
import DocumentManager from '../components/knowledge/DocumentManager.vue'
import RagSettings from '../components/knowledge/RagSettings.vue'
import { useKnowledge } from '../components/knowledge/useKnowledge'
import { KNOWLEDGE_CONTEXT_KEY } from '../components/knowledge/knowledgeContext'

defineOptions({ name: 'KnowledgeView' })

// 单一状态源：所有知识库域状态与方法在此创建，子组件经 inject 共享
const knowledge = useKnowledge()
provide(KNOWLEDGE_CONTEXT_KEY, knowledge)

onMounted(async () => {
  await knowledge.init()
})
</script>

<style scoped>
.knowledge-container {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-secondary);
}

.knowledge-header {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 24px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-color);
  flex-shrink: 0;
  height: var(--header-height);
  box-sizing: border-box;
}

.knowledge-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 24px 28px 32px;
  display: flex;
  flex-direction: column;
}

/* .back-btn 样式已抽取到 style.css 全局（F-1.15），此处不再重复 */

.header-title-group {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.knowledge-title {
  font-size: 17px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.2px;
}

.knowledge-subtitle {
  font-size: 12px;
  color: var(--text-muted);
  font-weight: 400;
}
</style>
