<!--
  SpeculativeDecodingSettings: 推测解码（加速推理）设置

  从 PerformanceSettings.vue 拆分而来，负责：
    - 默认推测配置 (spec_default)
    - 推测类型选择 (spec_type): draft-mtp / draft-eagle3 / ngram-* 等
    - Draft 模型参数: 路径、GPU层数、设备、线程数等
    - ngram 各模式参数: ngram-mod / ngram-simple / ngram-map-k / ngram-map-k4v
    - ngram-cache 缓存路径
-->
<template>
  <div class="advanced-section">
    <div class="advanced-header" @click="expanded = !expanded">
      <span class="advanced-icon">⚡</span>
      <span class="advanced-title">推测解码（加速推理）</span>
      <n-icon class="advanced-toggle" :component="expanded ? ChevronUp : ChevronDown" />
    </div>
    <n-collapse-transition>
      <div v-if="expanded" class="advanced-content">
        <!-- 默认推测配置 -->
        <n-form-item>
          <template #label>
            默认推测配置
            <HelpTip
              content="使用 llama.cpp 的默认推测解码配置，自动选择合适的参数。启用后其他推测参数将被忽略"
            />
          </template>
          <n-switch v-model:value="formConfig.spec_default" @update:value="autoSave" />
        </n-form-item>
        <n-text
          v-if="formConfig.spec_default"
          depth="3"
          style="font-size: 12px; margin-top: -12px; display: block; margin-bottom: 8px"
        >
          默认推测配置已启用，其他推测参数将被忽略
        </n-text>

        <!-- 推测类型 -->
        <n-form-item v-if="!formConfig.spec_default">
          <template #label>
            推测类型
            <HelpTip
              content="加速推理的推测解码技术。draft-mtp 需要模型内置 MTP 头（如 Qwen3.6-UD），draft-eagle3 需要 Eagle3 草稿模型，draft-dspark 需要带 Markov 头的 DSpark 草稿模型，ngram 类型对所有模型可用。自动模式下检测到 MTP 头会自动启用 draft-mtp，否则启用 ngram-mod"
            />
          </template>
          <n-select
            v-model:value="formConfig.spec_type"
            :options="specTypeOptions"
            placeholder="自动检测"
            clearable
          />
        </n-form-item>
        <n-text
          v-if="!settingsStore.modelCapabilities.has_mtp"
          depth="3"
          style="font-size: 12px; margin-top: -12px; display: block; margin-bottom: 8px"
        >
          当前模型不支持 MTP，draft-mtp 选项已隐藏
        </n-text>

        <!-- Draft 模型相关参数 -->
        <template
          v-if="
            !formConfig.spec_default &&
            (formConfig.spec_type === 'draft-eagle3' ||
              formConfig.spec_type === 'draft-dflash' ||
              formConfig.spec_type === 'draft-simple' ||
              formConfig.spec_type === 'draft-dspark')
          "
        >
          <n-text
            v-if="!formConfig.spec_draft_model"
            type="warning"
            style="font-size: 12px; display: block; margin-bottom: 8px"
          >
            未配置 Draft 模型路径，推测解码将无法工作
          </n-text>
          <n-form-item>
            <template #label>
              Draft 模型路径
              <HelpTip
                content="Eagle3/DFlash/Draft/DSpark 草稿模型的 .gguf 文件路径。draft-eagle3/draft-dflash/draft-simple/draft-dspark 模式需要"
              />
            </template>
            <n-input
              v-model:value="formConfig.spec_draft_model"
              placeholder="Eagle3/DFlash/Draft 草稿模型文件路径"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Draft GPU 层数
              <HelpTip
                content="草稿模型加载到 GPU 的层数。0=全部用 CPU，100=全部用 GPU。建议与主模型一致以保证加速效果"
              />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_draft_ngl"
              :min="0"
              :max="100"
              :step="1"
              placeholder="0"
              style="width: 100%"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Draft 设备
              <HelpTip content="草稿模型使用的 GPU 设备。留空自动选择，多卡场景可指定如 cuda:0" />
            </template>
            <n-input
              v-model:value="formConfig.spec_draft_device"
              placeholder="留空自动选择，如 cuda:0"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              最大 Draft Token 数
              <HelpTip
                content="每次推测最多生成的 token 数。值越大潜在加速越快但准确率下降，建议 3-4。DFlash 可达 block_size-1（通常 15）"
              />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_draft_n_max"
              :min="1"
              :max="15"
              :step="1"
              placeholder="3"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              最小 Draft Token 数
              <HelpTip
                content="每次推测最少生成的 token 数。0=不限制，设置后即使准确率低也会生成此数量的 token"
              />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_draft_n_min"
              :min="0"
              :max="15"
              :step="1"
              placeholder="0"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Draft 线程数
              <HelpTip content="Draft 模型使用的线程数。0=使用主模型线程数" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_draft_threads"
              :min="0"
              :max="256"
              :step="1"
              placeholder="0"
              style="width: 100%"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Draft 批处理线程数
              <HelpTip content="Draft 模型批处理时使用的线程数。0=使用主模型线程数" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_draft_threads_batch"
              :min="0"
              :max="256"
              :step="1"
              placeholder="0"
              style="width: 100%"
              @blur="autoSave"
            />
          </n-form-item>
        </template>

        <!-- draft-mtp 相关 -->
        <template v-if="!formConfig.spec_default && formConfig.spec_type === 'draft-mtp'">
          <n-form-item>
            <template #label>
              最大 Draft Token 数
              <HelpTip
                content="每次推测最多生成的 token 数。值越大潜在加速越快但准确率下降，建议 3-4"
              />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_draft_n_max"
              :min="1"
              :max="15"
              :step="1"
              placeholder="3"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              最小 Draft Token 数
              <HelpTip
                content="每次推测最少生成的 token 数。0=不限制，设置后即使准确率低也会生成此数量的 token"
              />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_draft_n_min"
              :min="0"
              :max="15"
              :step="1"
              placeholder="0"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Draft 线程数
              <HelpTip content="Draft 模型使用的线程数。0=使用主模型线程数" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_draft_threads"
              :min="0"
              :max="256"
              :step="1"
              placeholder="0"
              style="width: 100%"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Draft 批处理线程数
              <HelpTip content="Draft 模型批处理时使用的线程数。0=使用主模型线程数" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_draft_threads_batch"
              :min="0"
              :max="256"
              :step="1"
              placeholder="0"
              style="width: 100%"
              @blur="autoSave"
            />
          </n-form-item>
        </template>

        <!-- ngram-mod 相关 -->
        <template v-if="!formConfig.spec_default && formConfig.spec_type === 'ngram-mod'">
          <n-form-item>
            <template #label>
              ngram-mod 最小 token 数
              <HelpTip content="ngram-mod 推测的最小 n-gram 长度。值越小匹配越宽松，建议 48" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_mod_n_min"
              :min="1"
              :max="256"
              :step="1"
              placeholder="48"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              ngram-mod 最大 token 数
              <HelpTip content="ngram-mod 推测的最大 n-gram 长度。值越大覆盖范围越广，建议 64" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_mod_n_max"
              :min="1"
              :max="256"
              :step="1"
              placeholder="64"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              ngram-mod 查找长度
              <HelpTip content="ngram-mod 触发推测所需的最小匹配数。值越大越保守，建议 24" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_mod_n_match"
              :min="1"
              :max="256"
              :step="1"
              placeholder="24"
              @blur="autoSave"
            />
          </n-form-item>
        </template>

        <!-- ngram-simple 相关 -->
        <template v-if="!formConfig.spec_default && formConfig.spec_type === 'ngram-simple'">
          <n-form-item>
            <template #label>
              Size-N
              <HelpTip content="ngram-simple 的 n-gram 长度。控制匹配窗口大小，建议 64" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_simple_size_n"
              :min="1"
              :max="256"
              :step="1"
              placeholder="64"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Size-M
              <HelpTip content="ngram-simple 的 m-gram 长度。控制预测窗口大小，建议 64" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_simple_size_m"
              :min="1"
              :max="256"
              :step="1"
              placeholder="64"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Min-Hits
              <HelpTip content="ngram-simple 触发推测所需的最小命中次数。值越大越保守，建议 1" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_simple_min_hits"
              :min="1"
              :max="256"
              :step="1"
              placeholder="1"
              @blur="autoSave"
            />
          </n-form-item>
        </template>

        <!-- ngram-map-k 相关 -->
        <template v-if="!formConfig.spec_default && formConfig.spec_type === 'ngram-map-k'">
          <n-form-item>
            <template #label>
              Size-N
              <HelpTip content="ngram-map-k 的 n-gram 长度。控制匹配窗口大小，建议 64" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_map_k_size_n"
              :min="1"
              :max="256"
              :step="1"
              placeholder="64"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Size-M
              <HelpTip content="ngram-map-k 的 m-gram 长度。控制预测窗口大小，建议 64" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_map_k_size_m"
              :min="1"
              :max="256"
              :step="1"
              placeholder="64"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Min-Hits
              <HelpTip content="ngram-map-k 触发推测所需的最小命中次数。值越大越保守，建议 1" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_map_k_min_hits"
              :min="1"
              :max="256"
              :step="1"
              placeholder="1"
              @blur="autoSave"
            />
          </n-form-item>
        </template>

        <!-- ngram-map-k4v 相关 -->
        <template v-if="!formConfig.spec_default && formConfig.spec_type === 'ngram-map-k4v'">
          <n-form-item>
            <template #label>
              Size-N
              <HelpTip content="ngram-map-k4v 的 n-gram 长度。控制匹配窗口大小，建议 64" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_map_k4v_size_n"
              :min="1"
              :max="256"
              :step="1"
              placeholder="64"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Size-M
              <HelpTip content="ngram-map-k4v 的 m-gram 长度。控制预测窗口大小，建议 64" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_map_k4v_size_m"
              :min="1"
              :max="256"
              :step="1"
              placeholder="64"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              Min-Hits
              <HelpTip content="ngram-map-k4v 触发推测所需的最小命中次数。值越大越保守，建议 1" />
            </template>
            <n-input-number
              v-model:value="formConfig.spec_ngram_map_k4v_min_hits"
              :min="1"
              :max="256"
              :step="1"
              placeholder="1"
              @blur="autoSave"
            />
          </n-form-item>
        </template>

        <!-- ngram-cache 相关 -->
        <template v-if="!formConfig.spec_default && formConfig.spec_type === 'ngram-cache'">
          <n-form-item>
            <template #label>
              静态缓存路径
              <HelpTip
                content="预构建的静态 lookup 缓存文件路径。可显著加速推测解码，通常由训练工具生成"
              />
            </template>
            <n-input
              v-model:value="formConfig.lookup_cache_static"
              placeholder="lookup-cache-static 文件路径"
              @blur="autoSave"
            />
          </n-form-item>
          <n-form-item>
            <template #label>
              动态缓存路径
              <HelpTip
                content="运行时生成的动态 lookup 缓存文件路径。会在推理过程中自动更新，加速后续请求"
              />
            </template>
            <n-input
              v-model:value="formConfig.lookup_cache_dynamic"
              placeholder="lookup-cache-dynamic 文件路径"
              @blur="autoSave"
            />
          </n-form-item>
        </template>
      </div>
    </n-collapse-transition>
  </div>
</template>

<script setup lang="ts">
import { inject, ref } from 'vue'
import {
  NFormItem,
  NSelect,
  NInputNumber,
  NInput,
  NCollapseTransition,
  NText,
  NIcon
} from 'naive-ui'
import { ChevronDown, ChevronUp } from '@vicons/ionicons5'
import HelpTip from '../ui/HelpTip.vue'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'

defineOptions({ name: 'SpeculativeDecodingSettings' })

// 从父级注入配置上下文（formConfig、autoSave、specTypeOptions 等共享状态）
const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)!
// 从父级注入配置上下文（specTypeOptions 归入 performance 域）
const { core, performance } = ctx
const { formConfig, autoSave, settingsStore } = core
const { specTypeOptions } = performance

// 折叠状态：默认收起
const expanded = ref(false)
</script>

<style scoped>
/* 高级设置折叠区域 */
.advanced-section {
  margin-top: 16px;
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-lg);
  overflow: hidden;
}

.advanced-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: var(--bg-secondary);
  cursor: pointer;
  user-select: none;
  transition: background 0.2s;
}

.advanced-header:hover {
  background: var(--bg-hover);
}

.advanced-icon {
  color: var(--text-muted);
  flex-shrink: 0;
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
  transition: transform 0.2s;
}

.advanced-content {
  padding: 12px 16px;
  background: var(--bg-tertiary);
}
</style>
