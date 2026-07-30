<!--
  AIChatSettings: AI 对话设置
  整合系统提示词、生成参数、推理模式、朗读设置。
  专业路线：保持原生参数名，HelpTip 悬停显示解释。
-->
<template>
  <!-- ===== 系统提示词 ===== -->
  <n-form-item>
    <template #label>
      System Prompt
      <HelpTip
        content="追加在默认提示词后面的自定义指令，用于补充角色设定和行为约束。留空则仅使用默认提示词"
      />
    </template>
    <n-input
      v-model:value="formConfig.system_prompt"
      type="textarea"
      placeholder="自定义提示词将追加在豆芽默认提示词后面，用于补充角色设定和行为指令..."
      :autosize="{ minRows: 6, maxRows: 20 }"
      class="rounded-textarea"
      style="width: 100%"
      @blur="autoSave"
    />
  </n-form-item>

  <!-- ===== 推理模式 ===== -->
  <n-form-item>
    <template #label>
      推理模式
      <HelpTip
        content="控制模型的推理（思考）行为：on 始终开启推理，off 关闭推理，auto 由模型自行决定"
      />
    </template>
    <n-select
      v-model:value="formConfig.reasoning"
      :options="reasoningOptions"
      :disabled="!supportsReasoning"
    />
  </n-form-item>
  <n-text v-if="!supportsReasoning" depth="3" class="param-hint">当前模型不支持推理</n-text>
  <n-form-item>
    <template #label>
      推理预算
      <HelpTip content="限制推理过程的 token 数量。-1 无限，0 立即结束，N>0 限制为 N 个 token" />
    </template>
    <n-input-number
      v-model:value="formConfig.reasoning_budget"
      :min="-1"
      :step="1"
      placeholder="-1"
      style="width: 100%"
      :disabled="!supportsReasoning || formConfig.backend_sampling"
    />
  </n-form-item>
  <n-text v-if="formConfig.backend_sampling && supportsReasoning" depth="3" class="param-hint">
    后端采样与推理预算不兼容，推理预算已禁用
  </n-text>
  <n-form-item v-if="formConfig.reasoning_budget > 0">
    <template #label>
      预算耗尽消息
      <HelpTip content="推理预算耗尽时显示给用户的消息。留空则使用默认提示" />
    </template>
    <n-input
      v-model:value="formConfig.reasoning_budget_message"
      placeholder="推理预算耗尽时显示给用户的消息（留空使用默认提示）"
      :disabled="!supportsReasoning"
      @blur="autoSave"
    />
  </n-form-item>

  <!-- ===== 模型参考参数卡片 ===== -->
  <div v-if="currentModelRef" class="model-ref-card">
    <div class="model-ref-header">
      <span class="model-ref-icon">📋</span>
      <span class="model-ref-title">{{ currentModelRef.name }} 官方参考参数</span>
      <span v-if="settingsStore.currentModel" class="model-ref-current">
        当前: {{ settingsStore.currentModel }}
      </span>
    </div>
    <div v-if="currentModelRef.raw_thinking" class="model-ref-tabs">
      <n-button
        :type="!refShowThinking ? 'primary' : 'default'"
        ghost
        size="small"
        class="model-ref-tab"
        :class="{ active: !refShowThinking }"
        @click="refShowThinking = false"
      >
        普通模式
      </n-button>
      <n-button
        :type="refShowThinking ? 'primary' : 'default'"
        ghost
        size="small"
        class="model-ref-tab"
        :class="{ active: refShowThinking }"
        @click="refShowThinking = true"
      >
        思考模式
      </n-button>
    </div>
    <div class="model-ref-body">
      <template v-if="!refShowThinking">
        <div v-for="item in currentModelRef.params" :key="item.label" class="model-ref-row">
          <span class="model-ref-label">{{ item.label }}</span>
          <span class="model-ref-value">{{ item.value }}</span>
        </div>
      </template>
      <template v-else-if="currentModelRef.params_thinking">
        <div
          v-for="item in currentModelRef.params_thinking"
          :key="item.label"
          class="model-ref-row"
        >
          <span class="model-ref-label">{{ item.label }}</span>
          <span class="model-ref-value">{{ item.value }}</span>
        </div>
      </template>
      <div v-if="currentModelRef.note" class="model-ref-note">{{ currentModelRef.note }}</div>
    </div>
    <n-button type="primary" size="small" ghost class="model-ref-apply" @click="applyModelRef">
      应用参考
    </n-button>
  </div>

  <!-- ===== 核心生成参数 ===== -->
  <n-divider style="margin: 20px 0 12px" />
  <div class="section-label">生成参数</div>

  <n-form-item>
    <template #label>
      Temperature
      <HelpTip content="控制回答的随机性。值越低越确定保守，值越高越多样创意。推荐范围 0.3-0.8" />
    </template>
    <n-slider v-model:value="formConfig.temperature" :min="0" :max="2" :step="0.01" />
    <span class="slider-value">{{ formConfig.temperature }}</span>
    <n-button
      v-if="currentModelRef"
      type="primary"
      size="small"
      ghost
      class="reset-btn"
      @click="formConfig.temperature = activeModelRefRaw.temperature"
    >
      {{ activeModelRefRaw.temperature }}
    </n-button>
  </n-form-item>

  <n-form-item>
    <template #label>
      Top P
      <HelpTip
        content="核采样。从概率最高的候选词中筛选，只考虑累计概率达到此阈值的词。0.95 表示保留前 95% 概率的词"
      />
    </template>
    <n-slider v-model:value="formConfig.top_p" :min="0" :max="1" :step="0.01" />
    <span class="slider-value">{{ formConfig.top_p }}</span>
    <n-button
      v-if="currentModelRef"
      type="primary"
      size="small"
      ghost
      class="reset-btn"
      @click="formConfig.top_p = activeModelRefRaw.top_p"
    >
      {{ activeModelRefRaw.top_p }}
    </n-button>
  </n-form-item>

  <n-form-item>
    <template #label>
      Top K
      <HelpTip content="只从概率最高的 K 个候选词中选择。值越小选择越少越确定，0 表示不限制" />
    </template>
    <n-slider v-model:value="formConfig.top_k" :min="0" :max="100" :step="1" />
    <span class="slider-value">{{ formConfig.top_k }}</span>
    <n-button
      v-if="currentModelRef"
      type="primary"
      size="small"
      ghost
      class="reset-btn"
      @click="formConfig.top_k = activeModelRefRaw.top_k"
    >
      {{ activeModelRefRaw.top_k }}
    </n-button>
  </n-form-item>

  <n-form-item>
    <template #label>
      Context Size
      <HelpTip
        content="模型能记住的对话历史 token 数。值越大记忆越长但占用显存越多，超过模型支持的最大值会被自动截断"
      />
    </template>
    <n-slider
      v-model:value="contextSizeIndex"
      :min="0"
      :max="contextSizeSteps.length - 1"
      :step="1"
      :marks="contextSizeMarks"
    />
    <span class="slider-value">{{ formatContextSize(formConfig.context_size) }}</span>
    <n-button
      v-if="currentModelRef"
      type="primary"
      size="small"
      ghost
      class="reset-btn"
      @click="applyContextSizeRef"
    >
      {{ formatContextSize(activeModelRefRaw.context_size) }}
    </n-button>
  </n-form-item>

  <n-form-item>
    <template #label>
      Repeat Penalty
      <HelpTip content="大于 1 时惩罚重复内容，防止 AI 反复说同样的话。1.0 表示不惩罚" />
    </template>
    <n-slider v-model:value="formConfig.repeat_penalty" :min="0" :max="2" :step="0.01" />
    <span class="slider-value">{{ formConfig.repeat_penalty }}</span>
  </n-form-item>

  <!-- ===== 高级采样（折叠） ===== -->
  <div class="advanced-section">
    <div class="advanced-header" @click="advancedExpanded = !advancedExpanded">
      <span class="advanced-icon">⚙️</span>
      <span class="advanced-title">高级采样</span>
      <n-icon class="advanced-toggle" :component="advancedExpanded ? ChevronUp : ChevronDown" />
    </div>
    <n-collapse-transition>
      <div v-if="advancedExpanded" class="advanced-content">
        <n-form-item>
          <template #label>
            Min-P
            <HelpTip
              content="根据最高概率词动态过滤低概率词。0.05 表示过滤掉概率不到最高词 5% 的候选词"
            />
          </template>
          <n-slider v-model:value="formConfig.min_p" :min="0" :max="1" :step="0.01" />
          <span class="slider-value">{{ formConfig.min_p }}</span>
        </n-form-item>
        <n-form-item>
          <template #label>
            DRY Multiplier
            <HelpTip content="防止 AI 重复相同句式。0 表示关闭，大于 0 时值越强越不容易重复" />
          </template>
          <n-slider v-model:value="formConfig.dry_multiplier" :min="0" :max="5" :step="0.01" />
          <span class="slider-value">{{ formConfig.dry_multiplier }}</span>
        </n-form-item>
        <n-form-item v-if="formConfig.dry_multiplier > 0">
          <template #label>
            DRY Base
            <HelpTip content="DRY 采样的基础惩罚倍数。值越大对重复句式的惩罚越强，通常 1.0-2.0" />
          </template>
          <n-slider v-model:value="formConfig.dry_base" :min="1" :max="3" :step="0.01" />
          <span class="slider-value">{{ formConfig.dry_base }}</span>
        </n-form-item>
        <n-form-item v-if="formConfig.dry_multiplier > 0">
          <template #label>
            DRY Allowed Length
            <HelpTip
              content="允许重复的 token 序列长度。短于此长度的重复不会被惩罚，值越小越严格"
            />
          </template>
          <n-slider v-model:value="formConfig.dry_allowed_length" :min="1" :max="10" :step="1" />
          <span class="slider-value">{{ formConfig.dry_allowed_length }}</span>
        </n-form-item>
        <n-form-item v-if="formConfig.dry_multiplier > 0">
          <template #label>
            Sequence Breaker
            <HelpTip
              content="DRY 采样的序列中断符。遇到这些字符时重置 DRY 惩罚。多个中断符用逗号分隔，如 \n,。,！,？"
            />
          </template>
          <n-input
            v-model:value="formConfig.dry_sequence_breaker"
            placeholder="如：\n,。,？（逗号分隔）"
            @blur="autoSave"
          />
        </n-form-item>
        <n-form-item v-if="formConfig.dry_multiplier > 0">
          <template #label>
            Penalty Last N
            <HelpTip content="DRY 惩罚的回看窗口大小（token 数）。0 使用 repeat_last_n 的值" />
          </template>
          <n-input-number
            v-model:value="formConfig.dry_penalty_last_n"
            :min="0"
            :max="2048"
            :step="1"
            placeholder="0"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>
        <n-form-item>
          <template #label>
            Samplers
            <HelpTip
              content="手动指定采样器的执行顺序，用逗号分隔，如 top_k,top_p,temperature。留空使用后端默认"
            />
          </template>
          <n-input
            v-model:value="formConfig.samplers"
            placeholder="如：top_k,top_p,temperature（留空使用默认）"
            @blur="autoSave"
          />
        </n-form-item>
        <n-form-item label="Ignore EOS">
          <n-checkbox v-model:checked="formConfig.ignore_eos" @update:value="autoSave" />
          <span class="setting-hint">开启后即使遇到结束符也继续生成，可用于强制生成长文本</span>
        </n-form-item>
        <n-form-item>
          <template #label>
            Adaptive Target
            <HelpTip
              content="请求级自适应采样的目标熵值（0-1）。0 禁用，设置后模型会动态调整采样参数以逼近该目标"
            />
          </template>
          <n-input-number
            v-model:value="formConfig.adaptive_target"
            :min="0"
            :max="1"
            :step="0.01"
            placeholder="0"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>
        <n-form-item>
          <template #label>
            Adaptive Decay
            <HelpTip
              content="请求级自适应采样的衰减系数（0-1）。0 禁用，控制自适应调整的收敛速度"
            />
          </template>
          <n-input-number
            v-model:value="formConfig.adaptive_decay"
            :min="0"
            :max="1"
            :step="0.01"
            placeholder="0"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>
      </div>
    </n-collapse-transition>
  </div>

  <!-- ===== 朗读设置 ===== -->
  <n-divider style="margin: 24px 0 12px" />
  <div class="section-label">朗读设置</div>
  <TTSSettings />
</template>

<script setup lang="ts">
import { ref, inject } from 'vue'
import {
  NFormItem,
  NInput,
  NSelect,
  NInputNumber,
  NSlider,
  NCheckbox,
  NText,
  NCollapseTransition,
  NIcon,
  NButton,
  NDivider
} from 'naive-ui'
import { ChevronDown, ChevronUp } from '@vicons/ionicons5'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import HelpTip from '../ui/HelpTip.vue'
import TTSSettings from './TTSSettings.vue'

defineOptions({ name: 'AIChatSettings' })

const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)
if (!ctx) {
  throw new Error('AIChatSettings 必须在 SettingsView 内使用（缺少 settingsContext provide）')
}
const {
  formConfig,
  autoSave,
  reasoningOptions,
  supportsReasoning,
  currentModelRef,
  activeModelRefRaw,
  refShowThinking,
  applyModelRef,
  contextSizeIndex,
  contextSizeSteps,
  contextSizeMarks,
  formatContextSize,
  applyContextSizeRef,
  settingsStore
} = ctx

const advancedExpanded = ref(false)
</script>

<style scoped>
.slider-value {
  min-width: 50px;
  text-align: right;
  font-size: 13px;
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
}
.param-hint {
  font-size: 12px;
  margin-top: -12px;
  display: block;
  margin-bottom: 8px;
}
.reset-btn {
  margin-left: 4px;
  min-width: 32px;
}
.setting-hint {
  margin-left: 12px;
  font-size: 12px;
  color: var(--text-muted);
}
.section-label {
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-muted);
  margin-bottom: 8px;
}

/* 模型参考卡片 */
.model-ref-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 16px;
  margin-bottom: 16px;
}
.model-ref-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.model-ref-icon {
  font-size: 20px;
}
.model-ref-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  flex: 1;
}
.model-ref-current {
  font-size: 12px;
  color: var(--text-muted);
  background: var(--bg-tertiary);
  padding: 2px 8px;
  border-radius: 4px;
}
.model-ref-tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
.model-ref-tab {
  flex: 1;
}
.model-ref-tab.active {
  color: var(--accent-primary);
  border-color: var(--accent-primary);
}
.model-ref-body {
  margin-bottom: 12px;
}
.model-ref-row {
  display: flex;
  justify-content: space-between;
  padding: 6px 0;
  border-bottom: 1px solid var(--border-light);
}
.model-ref-row:last-child {
  border-bottom: none;
}
.model-ref-label {
  font-size: 13px;
  color: var(--text-secondary);
}
.model-ref-value {
  font-size: 13px;
  color: var(--text-primary);
  font-weight: 500;
}
.model-ref-note {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 8px;
  padding: 8px;
  background: var(--bg-tertiary);
  border-radius: 6px;
}
.model-ref-apply {
  width: 100%;
}

/* 高级采样折叠 */
.advanced-section {
  margin-top: 16px;
  border: 1px solid var(--border-color);
  border-radius: 12px;
  overflow: hidden;
}
.advanced-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  background: var(--bg-secondary);
  cursor: pointer;
  user-select: none;
  transition: background 0.2s;
}
.advanced-header:hover {
  background: var(--bg-hover);
}
.advanced-icon {
  font-size: 16px;
}
.advanced-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  flex: 1;
}
.advanced-toggle {
  font-size: 16px;
  color: var(--text-muted);
}
.advanced-content {
  padding: 12px 16px;
  background: var(--bg-tertiary);
}

.rounded-textarea {
  border-radius: 8px;
}
</style>
