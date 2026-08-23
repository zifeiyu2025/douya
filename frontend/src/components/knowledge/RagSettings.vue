<!--
  RagSettings: RAG 检索设置区（C-6 自 KnowledgeView 拆分）
  启用开关（与联网搜索互斥）、检索参数、分块参数、嵌入模型配置与保存。
-->
<template>
  <div class="kb-section">
    <div class="kb-section-header">
      <span class="section-icon">⚙️</span>
      <span class="section-title">RAG 设置</span>
    </div>

    <n-form label-placement="left" label-width="96" :model="ragConfig" class="rag-form">
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
            <div class="embedding-status">
              <span v-if="ragConfig.embeddingModel" class="status-active">
                ✓ 使用专用嵌入模型：{{ ragConfig.embeddingModel }}
              </span>
              <span v-else class="status-fallback">
                ⚠ 未配置专用嵌入模型，将使用聊天模型（检索质量可能较差）
              </span>
            </div>
            <div class="embedding-recommend">
              <span class="recommend-label">推荐模型：</span>
              <span class="recommend-tags">
                <span class="recommend-tag" @click="selectRecommendedModel('nomic-embed-text')">
                  nomic-embed-text
                </span>
                <span class="recommend-tag" @click="selectRecommendedModel('bge-base-en-v1.5')">
                  bge-base-en-v1.5
                </span>
                <span class="recommend-tag" @click="selectRecommendedModel('bge-large-zh-v1.5')">
                  bge-large-zh-v1.5
                </span>
              </span>
            </div>
            <div class="embedding-tip">💡 专用嵌入模型更快、更准确。留空则使用当前聊天模型。</div>
          </div>
        </div>
      </n-form-item>
    </n-form>

    <div class="rag-save-row">
      <n-button
        type="primary"
        size="small"
        :loading="savingRAG"
        class="save-btn"
        @click="handleSaveRAGConfig"
      >
        <template #icon>
          <n-icon size="16"><CheckmarkCircleOutline /></n-icon>
        </template>
        保存设置
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import {
  NButton,
  NIcon,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSlider,
  NSwitch
} from 'naive-ui'
import { CheckmarkCircleOutline } from '@vicons/ionicons5'
import { KNOWLEDGE_CONTEXT_KEY, type KnowledgeContext } from './knowledgeContext'

defineOptions({ name: 'RagSettings' })

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
.rag-form {
  margin-bottom: 4px;
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

.slider-mark {
  min-width: 36px;
  text-align: center;
  font-size: 13px;
  font-weight: 600;
  color: var(--accent-primary);
  background: var(--accent-tertiary);
  padding: 2px 8px;
  border-radius: var(--border-radius-sm);
}

.rag-input {
  width: 120px;
}

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

.embedding-help {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  background: var(--bg-tertiary);
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.6;
}

.embedding-status {
  display: flex;
  align-items: center;
  gap: 4px;
}

/* RAG 设置状态色：启用 → --accent-g-primary */
.status-active {
  color: var(--accent-g-primary);
  font-weight: 500;
}

/* RAG 设置状态色：禁用/未配置 → --text-muted */
.status-fallback {
  color: var(--text-muted);
  font-weight: 500;
}

.embedding-recommend {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.recommend-label {
  color: var(--text-secondary);
  font-weight: 500;
}

.recommend-tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.recommend-tag {
  padding: 2px 8px;
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
  font-family: monospace;
  font-size: 11px;
}

.recommend-tag:hover {
  background: var(--accent-primary);
  color: white;
  border-color: var(--accent-primary);
}

.embedding-tip {
  color: var(--text-tertiary);
  font-size: 11px;
}

.rag-save-row {
  display: flex;
  justify-content: flex-end;
  padding-top: 4px;
  border-top: 1px solid var(--border-light);
}

.save-btn {
  font-weight: 600;
}
</style>
