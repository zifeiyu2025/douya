<!--
  ParamsPanel: 聊天页采样参数快捷抽屉
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
/* 来源说明：左缘苔绿细线的信息行，无色块底 */
.params-source-line {
  margin-bottom: 16px;
  padding: 6px 0 6px 12px;
  font-size: 12px;
  color: var(--text-secondary);
  background: transparent;
  border-left: 2px solid color-mix(in srgb, var(--accent-primary) 40%, transparent);
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

/* 参数名：衬线体如词条标题 */
.param-name {
  font-family: var(--font-display);
  font-size: 13px;
  letter-spacing: 0.02em;
  color: var(--text-primary);
}

.slider-value {
  min-width: 44px;
  margin-left: auto;
  text-align: right;
  font-family: var(--font-mono);
  font-size: 12.5px;
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
}

/* 推荐角标：小号方角印章（hairline 苔绿描边），点击回官方推荐值 */
.ref-chip {
  padding: 1px 7px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--accent-primary);
  border: 1px solid color-mix(in srgb, var(--accent-primary) 55%, transparent);
  border-radius: var(--border-radius-sm);
  cursor: pointer;
  transition:
    background-color 0.15s ease,
    color 0.15s ease,
    border-color 0.15s ease;
}

/* hover 落实苔绿底：强调色底上的字色统一走纸面底色令牌 */
.ref-chip:hover {
  background: var(--accent-primary);
  border-color: var(--accent-primary);
  color: var(--bg-primary);
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
