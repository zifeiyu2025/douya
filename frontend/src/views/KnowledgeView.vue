<!--
  KnowledgeView: 知识库管理视图 ·「档案柜」书房版式
  版式隐喻：整页是一格档案柜 ——
    柜铭（衬线标题 + 引言副题）→ 一条细工具栏（选卷 / 新建 / 删除）
    → 正文两节：§一 档案目录（DocumentManager）、§二 检索配置（RagSettings）。
  状态与逻辑集中在 useKnowledge 单一状态源，经 provide 注入子组件。
-->
<template>
  <div class="knowledge-container">
    <!-- 柜铭：返回 + 衬线标题 -->
    <header class="kb-masthead">
      <button class="masthead-back" type="button" aria-label="返回" @click="$router.push('/')">
        <AppIcon name="back" :size="17" />
      </button>
      <div class="masthead-text">
        <h1 class="kb-title">知识库</h1>
        <p class="kb-motto">卷宗归档之处——上传文档，编目成册，供问答时检索征引。</p>
      </div>
    </header>

    <!-- 细工具栏：知识库切换 / 新建 / 删除（panel 表面 + hairline 底边，由子组件自带） -->
    <KBSelector />

    <!-- 正文滚动区：两节目录各自限宽居中 -->
    <div class="kb-body">
      <DocumentManager />
      <RagSettings />
    </div>
  </div>
</template>

<script setup lang="ts">
import { provide, onMounted } from 'vue'
import AppIcon from '../components/ui/AppIcon.vue'
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
  /* B5 veil 结构层：容器统一铺一层结构色，内部区块不再叠底
   * （避免半透明平方效应闷死背景图） */
  background: var(--surface-veil);
}

/* ===== 柜铭 ===== */
.kb-masthead {
  display: flex;
  align-items: center;
  gap: 16px;
  /* 顶部净空沿用既有约定（避让拖拽带），底边为柜铭与工具栏的章节界 */
  padding: 62px 32px 18px;
  border-bottom: 1px solid var(--border-light);
  flex-shrink: 0;
}

/* 返回钮：素面方钮，悬浮才着一阶纸色 */
.masthead-back {
  width: 30px;
  height: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: var(--border-radius-sm);
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
}

.masthead-back:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.masthead-text {
  min-width: 0;
}

.kb-title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 20px;
  font-weight: 600;
  letter-spacing: 0.06em;
  line-height: 1.3;
  color: var(--text-primary);
}

.kb-motto {
  margin: 2px 0 0;
  font-family: var(--font-display);
  font-size: 12.5px;
  letter-spacing: 0.02em;
  color: var(--text-secondary);
}

/* ===== 正文滚动区 ===== */
.kb-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 28px 32px 64px;
}
</style>
