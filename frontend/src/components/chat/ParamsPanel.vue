<!--
  ParamsPanel: 聊天页采样参数快捷抽屉（改进计划 C-4 第④项）
  六滑块 + 官方推荐角标 + 一键回推荐；与设置页共享同一份 config
  （写入管线见 useSamplingSettings.ts），改动自动保存。
-->
<template>
  <n-drawer
    :show="isOpen"
    :width="380"
    placement="right"
    @update:show="(v: boolean) => !v && closeParamsPanel()"
  >
    <n-drawer-content title="生成参数" closable>
      <!-- 当前模型推荐来源说明 -->
      <div class="params-source-line">
        <template v-if="matchedModelRef">
          角标为 {{ matchedModelRef.name }} 官方推荐值，点击即回推荐
        </template>
        <template v-else>当前模型无官方推荐参数，可自由调节</template>
      </div>

      <!-- 六滑块 -->
      <div v-for="def in SAMPLER_SLIDERS" :key="def.key" class="param-row">
        <div class="param-label">
          <span class="param-name">{{ def.label }}</span>
          <HelpTip :content="def.tip" />
          <span
            v-if="def.recommendable && recommendedRaw"
            class="ref-chip"
            :title="`设回 ${matchedModelRef?.name ?? ''} 官方推荐值`"
            @click="setToRecommended(def.key)"
          >
            {{ recommendedRaw[def.key as RecommendableKey] }}
          </span>
          <span class="slider-value">{{ draft[def.key] }}</span>
        </div>
        <n-slider
          v-model:value="draft[def.key]"
          :min="def.min"
          :max="def.max"
          :step="def.step"
          @update:value="scheduleFlush"
        />
      </div>

      <n-button
        v-if="recommendedRaw"
        size="small"
        type="primary"
        ghost
        class="apply-all-btn"
        @click="applyAllRecommended"
      >
        全部回推荐值
      </n-button>

      <div class="params-footer-hint">改动会自动保存并即时生效</div>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import { NDrawer, NDrawerContent, NSlider, NButton, useMessage } from 'naive-ui'
import HelpTip from '../ui/HelpTip.vue'
import {
  useSamplingSettings,
  SAMPLER_SLIDERS,
  type RecommendableKey
} from '../../composables/useSamplingSettings'

const {
  isOpen,
  draft,
  lastError,
  matchedModelRef,
  recommendedRaw,
  scheduleFlush,
  closeParamsPanel,
  setToRecommended,
  applyAllRecommended
} = useSamplingSettings()

// composable 不碰 UI，保存失败经 lastError 传到这里弹 toast
const message = useMessage()
watch(lastError, msg => {
  if (msg) {
    message.error(msg)
    lastError.value = ''
  }
})
</script>

<style scoped>
.params-source-line {
  margin-bottom: 16px;
  padding: 8px 12px;
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  border-radius: var(--border-radius-sm);
}

.param-row {
  margin-bottom: 18px;
}

.param-label {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
}

.param-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.slider-value {
  min-width: 44px;
  margin-left: auto;
  text-align: right;
  font-size: 13px;
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
}

/* 推荐角标：小号描边胶囊，点击回官方推荐值 */
.ref-chip {
  padding: 1px 7px;
  font-size: 11px;
  color: var(--accent-primary);
  border: 1px solid var(--accent-primary);
  border-radius: 10px;
  cursor: pointer;
  transition:
    background 0.15s ease,
    color 0.15s ease;
}

.ref-chip:hover {
  background: var(--accent-primary);
  color: #fff;
}

.apply-all-btn {
  width: 100%;
  margin-top: 4px;
}

.params-footer-hint {
  margin-top: 14px;
  font-size: 12px;
  color: var(--text-muted);
  text-align: center;
}
</style>
