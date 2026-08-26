<!--
  RagSettings: 检索配置区（自 KnowledgeView 拆分）
  启用开关（与联网搜索互斥）、检索参数、分块参数、嵌入模型配置与保存。
  对外契约保持不变：无 props、无 emits，依赖 KnowledgeView 的 provide 上下文。
-->
<template>
  <section class="rag-section">
    <!-- 节头：§ 编号 + 标题 -->
    <header class="rag-head">
      <span class="section-num head-no">§ 二</span>
      <h2 class="head-title">检索配置</h2>
      <span class="head-space" />
    </header>

    <n-form
      label-placement="left"
      label-width="96"
      :model="ragConfig"
      :show-feedback="false"
      class="rag-form"
    >
      <n-form-item label="启用知识库">
        <n-switch v-model:value="ragConfig.enabled" @update:value="handleRAGToggle" />
      </n-form-item>
      <n-form-item label="检索数量">
        <div class="slider-wrap">
          <n-slider v-model:value="ragConfig.topK" :min="1" :max="10" :step="1" />
          <span class="slider-mark">{{ ragConfig.topK }}</span>
        </div>
      </n-form-item>
      <n-form-item label="最低相关性">
        <div class="slider-wrap">
          <n-slider v-model:value="ragConfig.minScore" :min="0" :max="1" :step="0.05" />
          <span class="slider-mark">{{ ragConfig.minScore.toFixed(2) }}</span>
        </div>
      </n-form-item>
      <n-form-item label="分块大小">
        <n-input-number
          v-model:value="ragConfig.chunkSize"
          :min="128"
          :max="4096"
          :step="64"
          size="small"
          class="rag-input"
        />
      </n-form-item>
      <n-form-item label="分块重叠">
        <n-input-number
          v-model:value="ragConfig.chunkOverlap"
          :min="0"
          :max="512"
          :step="16"
          size="small"
          class="rag-input"
        />
      </n-form-item>
      <n-form-item label="嵌入模型">
        <div class="embedding-config">
          <n-select
            v-model:value="selectedEmbeddingModel"
            :options="embeddingModelOptions"
            placeholder="从列表选择嵌入模型"
            size="small"
            clearable
            class="embedding-select"
            @update:value="onEmbeddingModelSelect"
          />
          <n-input
            v-model:value="manualEmbeddingModel"
            placeholder="或手动输入模型名称"
            size="small"
            class="embedding-input"
            @update:value="onManualEmbeddingModelInput"
          />
          <div class="embedding-help">
            <!-- 状态行以印章方点替代符号图标 -->
            <p class="embed-status">
              <span class="status-seal" :class="ragConfig.embeddingModel ? 'is-set' : 'is-unset'" />
              <template v-if="ragConfig.embeddingModel">
                已配置专用嵌入模型：
                <span class="embed-model">{{ ragConfig.embeddingModel }}</span>
              </template>
              <template v-else>未配置专用嵌入模型，将使用聊天模型（检索质量可能较差）</template>
            </p>
            <p class="embed-recommend">
              <span class="recommend-label">推荐模型：</span>
              <span class="recommend-tags">
                <button
                  v-for="name in RECOMMENDED_MODELS"
                  :key="name"
                  type="button"
                  class="recommend-tag"
                  @click="selectRecommendedModel(name)"
                >
                  {{ name }}
                </button>
              </span>
            </p>
            <p class="embed-tip">
              <AppIcon name="bulb" :size="12" class="tip-icon" />
              专用嵌入模型更快、更准确。留空则使用当前聊天模型。
            </p>
          </div>
        </div>
      </n-form-item>
    </n-form>

    <footer class="rag-save-row">
      <n-button type="primary" size="small" :loading="savingRAG" @click="handleSaveRAGConfig">
        <template #icon>
          <AppIcon name="check" :size="14" />
        </template>
        保存设置
      </n-button>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import {
  NButton,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSlider,
  NSwitch
} from 'naive-ui'
import AppIcon from '../ui/AppIcon.vue'
import { KNOWLEDGE_CONTEXT_KEY, type KnowledgeContext } from './knowledgeContext'

defineOptions({ name: 'RagSettings' })

/** 推荐嵌入模型清单（点击即回填） */
const RECOMMENDED_MODELS = ['nomic-embed-text', 'bge-base-en-v1.5', 'bge-large-zh-v1.5'] as const

const ctx = inject<KnowledgeContext>(KNOWLEDGE_CONTEXT_KEY)
if (!ctx) {
  throw new Error('RagSettings 必须在 KnowledgeView 内使用（缺少 knowledgeContext provide）')
}
// 本组件仅负责：RAG 开关/检索参数/分块参数/嵌入模型选择与保存
const {
  ragConfig,
  savingRAG,
  selectedEmbeddingModel,
  manualEmbeddingModel,
  embeddingModelOptions
} = ctx
const {
  handleRAGToggle,
  handleSaveRAGConfig,
  onEmbeddingModelSelect,
  onManualEmbeddingModelInput,
  selectRecommendedModel
} = ctx
</script>

<style scoped>
.rag-section {
  max-width: 880px;
  margin: 52px auto 0;
}

/* ===== 节头（与文档列表同款章节界） ===== */
.rag-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--border-light);
}

.head-title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.05em;
  color: var(--text-primary);
}

.head-space {
  flex: 1;
}

/* ===== 表单 ===== */
.rag-form :deep(.n-form-item) {
  margin-bottom: 16px;
}

.slider-wrap {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
}

.slider-wrap .n-slider {
  flex: 1;
}

/* 数值批注：等宽字 + 右对齐，不着底色 */
.slider-mark {
  min-width: 38px;
  text-align: right;
  font-family: var(--font-mono);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--accent-primary);
}

.rag-input {
  width: 120px;
}

/* ===== 嵌入模型 ===== */
.embedding-config {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.embedding-select,
.embedding-input {
  width: 100%;
}

/* 批注纸：三级纸面 + hairline 边框，无投影 */
.embedding-help {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-sm);
  font-size: 12px;
  line-height: 1.6;
}

.embed-status,
.embed-recommend,
.embed-tip {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin: 0;
}

.status-seal {
  width: 5px;
  height: 5px;
  border-radius: 1px;
  flex-shrink: 0;
}

/* 已配置=苔绿 / 未配置=赭石（延续印章方点语言） */
.status-seal.is-set {
  background: var(--accent-primary);
}

.status-seal.is-unset {
  background: var(--accent-warning);
}

.embed-model {
  font-family: var(--font-mono);
  font-size: 11.5px;
  color: var(--accent-success-g-primary);
}

.recommend-label {
  color: var(--text-secondary);
}

.recommend-tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

/* 推荐模型签：等宽小字 + hairline 边框，悬浮仅着苔绿描边与浅纸底 */
.recommend-tag {
  padding: 2px 8px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-secondary);
  background: var(--surface-panel);
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-xs);
  cursor: pointer;
  transition:
    color var(--transition-fast),
    border-color var(--transition-fast),
    background var(--transition-fast);
}

.recommend-tag:hover {
  color: var(--accent-primary);
  border-color: var(--accent-primary);
  background: var(--accent-tertiary);
}

.embed-tip {
  color: var(--text-tertiary);
  font-size: 11px;
}

.tip-icon {
  color: var(--text-tertiary);
  flex-shrink: 0;
}

/* ===== 保存行 ===== */
.rag-save-row {
  display: flex;
  justify-content: flex-end;
  padding-top: 14px;
  border-top: 1px solid var(--border-light);
}
</style>
