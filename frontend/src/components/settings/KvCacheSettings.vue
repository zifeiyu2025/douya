<!--
  KvCacheSettings: KV 缓存与高级运行参数设置
  生活类比：像汽车的变速箱调校——控制内存映射、缓存精度、多GPU分割等"底层机械"参数

  从 PerformanceSettings.vue 拆分而来，负责：
    - 内存映射 (mmap)
    - KV 缓存 K/V 类型与卸载
    - 上下文移位
    - 多 GPU 分割（split-mode / tensor-split / main-gpu）
    - 并发槽位、线程数、批处理大小
    - MoE 权重 CPU 卸载
    - 分组注意力
    - 缓存复用、空闲休眠等
-->
<template>
  <div class="advanced-section">
    <div class="advanced-header" @click="expanded = !expanded">
      <span class="advanced-icon">💾</span>
      <span class="advanced-title">KV 缓存</span>
      <n-icon class="advanced-toggle" :component="expanded ? ChevronUp : ChevronDown" />
    </div>
    <n-collapse-transition>
      <div v-if="expanded" class="advanced-content">
        <!-- 内存映射 -->
        <n-form-item>
          <template #label>
            内存映射 (mmap)
            <HelpTip
              content="将模型文件映射到内存而非全部加载。开启可加快启动速度、减少内存占用，关闭则全部预加载到内存"
            />
          </template>
          <n-switch v-model:value="formConfig.mmap" />
        </n-form-item>

        <!-- KV 缓存 K 类型 -->
        <n-form-item>
          <template #label>
            KV 缓存 K 类型
            <HelpTip
              content="Key 缓存的量化精度。K 决定注意力查找方向，建议用高精度（q8_0）。选「自动」由系统根据显存自动选择"
            />
          </template>
          <n-select
            v-model:value="formConfig.cache_type_k"
            :options="cacheTypeKOptions"
            placeholder="自动（q8_0）"
            clearable
          />
        </n-form-item>

        <!-- KV 缓存 V 类型 -->
        <n-form-item>
          <template #label>
            KV 缓存 V 类型
            <HelpTip
              content="Value 缓存的量化精度。V 是实际内容，可以更激进压缩。选「自动」由系统智能选择"
            />
          </template>
          <n-select
            v-model:value="formConfig.cache_type_v"
            :options="cacheTypeVOptions"
            placeholder="自动（q4_0）"
            clearable
          />
        </n-form-item>

        <!-- KV 缓存卸载 -->
        <n-form-item>
          <template #label>
            KV 缓存卸载
            <HelpTip
              content="开启后 KV 缓存保留在显卡显存中，逐 token 生成速度更快，但会增加显存占用（默认开启，与 llama.cpp 原生一致）。关闭则 KV 缓存放回内存，更省显存但生成明显变慢，一般无需关闭；显存不足时可临时关闭或依赖自动缩小上下文"
            />
          </template>
          <n-switch v-model:value="formConfig.kv_offload" />
        </n-form-item>

        <!-- 上下文移位 -->
        <n-form-item>
          <template #label>
            上下文移位
            <HelpTip
              content="当对话超出上下文长度时，自动移除最早的内容腾出空间，而非直接报错。作为应用层压缩的兜底，建议保持开启"
            />
          </template>
          <n-switch v-model:value="formConfig.context_shift" />
        </n-form-item>

        <!-- 检查点最小步长 -->
        <n-form-item>
          <template #label>
            检查点最小步长
            <HelpTip
              content="上下文检查点之间的最小 token 步数。0=使用默认值，设置后每隔指定步数保存一次检查点，便于回溯和 KV 缓存复用"
            />
          </template>
          <n-input-number
            v-model:value="formConfig.checkpoint_min_step"
            :min="0"
            :max="1000"
            :step="1"
            placeholder="0"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>

        <!-- GPU 设备 -->
        <n-form-item>
          <template #label>
            GPU 设备
            <HelpTip content="指定使用的 GPU 设备。留空自动选择，多卡场景用逗号分隔（如 0,1）" />
          </template>
          <n-input v-model:value="formConfig.device" placeholder="留空自动选择，多卡如 0,1" />
        </n-form-item>

        <!-- 多 GPU 分割模式 -->
        <n-form-item>
          <template #label>
            多 GPU 分割模式
            <HelpTip
              content="多卡并行时张量在各 GPU 间的分割方式。layer=按层分割（默认），row=按行分割，tensor=按张量分割，none=禁用多卡。多卡用户建议搭配下方张量分割使用"
            />
          </template>
          <n-select
            v-model:value="formConfig.split_mode"
            :options="splitModeOptions"
            placeholder="使用 llama.cpp 默认（layer）"
            clearable
          />
        </n-form-item>

        <!-- 张量分割权重 -->
        <n-form-item>
          <template #label>
            张量分割权重
            <HelpTip
              content="多卡显存分配权重（逗号分隔），如 3,1 表示第一块卡分 75%、第二块 25%。需与 GPU 设备数量一致"
            />
          </template>
          <n-input
            v-model:value="formConfig.tensor_split"
            placeholder="如 3,1（两张卡）"
            :disabled="formConfig.split_mode === 'none'"
          />
        </n-form-item>
        <n-text
          v-if="formConfig.split_mode === 'none' && formConfig.tensor_split"
          depth="3"
          style="font-size: 12px; margin-top: -12px; display: block; margin-bottom: 8px"
        >
          分割模式为 none 时张量分割权重无效
        </n-text>

        <!-- 主 GPU -->
        <n-form-item>
          <template #label>
            主 GPU
            <HelpTip content="多卡场景指定主计算 GPU（索引从 0 开始）。-1=自动选择，0=第一块卡" />
          </template>
          <n-input-number
            v-model:value="formConfig.main_gpu"
            :min="-1"
            :step="1"
            placeholder="-1 = 自动"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>

        <!-- 并发槽位数 -->
        <n-form-item>
          <template #label>
            并发槽位数
            <HelpTip
              content="同时处理的请求数。0=自动（通常为 1），增大可支持多用户并发但会按比例增加显存占用"
            />
          </template>
          <n-input-number
            v-model:value="formConfig.parallel"
            :min="0"
            placeholder="0 = 自动"
            style="width: 100%"
          />
        </n-form-item>

        <!-- CPU 线程数 -->
        <n-form-item>
          <template #label>
            CPU 线程数
            <HelpTip
              content="推理使用的 CPU 线程数。0=自动（按 CPU 核心数），降低可避免 CPU 过载"
            />
          </template>
          <n-input-number
            v-model:value="formConfig.threads"
            :min="0"
            :step="1"
            placeholder="0 = 自动"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>

        <!-- 批处理大小 -->
        <n-form-item>
          <template #label>
            批处理大小
            <HelpTip
              content="每次前向传播处理的 token 数（batch size）。0=自动，增大可提升吞吐但增加显存/内存占用"
            />
          </template>
          <n-input-number
            v-model:value="formConfig.batch_size"
            :min="0"
            :step="64"
            placeholder="0 = 自动"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>

        <!-- MoE 权重 CPU 卸载 -->
        <n-form-item>
          <template #label>
            MoE CPU 卸载
            <HelpTip
              content="将 MoE 模型的专家权重保留在 CPU 内存，仅激活的专家加载到 GPU，显著降低显存占用（适用于混元/DeepSeek 等 MoE 大模型）"
            />
          </template>
          <n-switch v-model:value="formConfig.cpu_moe" @update:value="autoSave" />
        </n-form-item>

        <!-- MoE 前 N 层卸载 -->
        <n-form-item>
          <template #label>
            MoE 前 N 层卸载
            <HelpTip
              content="仅将前 N 层 MoE 专家权重保留在 CPU，0=不启用。比全局 cpu_moe 更精细，兼顾显存与性能"
            />
          </template>
          <n-input-number
            v-model:value="formConfig.n_cpu_moe"
            :min="0"
            :step="1"
            placeholder="0 = 不启用"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>

        <!-- 分组注意力 N -->
        <n-form-item>
          <template #label>
            分组注意力 N
            <HelpTip
              content="将上下文分成 N 组进行注意力计算，用于超长文本生成。0=禁用，需同时设置 W 才生效"
            />
          </template>
          <n-input-number
            v-model:value="formConfig.grp_attn_n"
            :min="0"
            :max="128"
            :step="1"
            placeholder="0"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>

        <!-- 分组注意力 W -->
        <n-form-item>
          <template #label>
            分组注意力 W
            <HelpTip content="分组注意力的窗口宽度（token 数）。0=禁用，需同时设置 N 才生效" />
          </template>
          <n-input-number
            v-model:value="formConfig.grp_attn_w"
            :min="0"
            :max="131072"
            :step="512"
            placeholder="0"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>
        <n-text
          v-if="
            (formConfig.grp_attn_n > 0 && formConfig.grp_attn_w === 0) ||
            (formConfig.grp_attn_w > 0 && formConfig.grp_attn_n === 0)
          "
          depth="3"
          style="font-size: 12px; margin-top: -12px; display: block; margin-bottom: 8px"
        >
          分组注意力需要同时设置 N 和 W 才能生效
        </n-text>

        <!-- 缓存复用块大小 -->
        <n-form-item>
          <template #label>
            缓存复用块大小
            <HelpTip
              content="KV 缓存复用的块大小。0=禁用，256=推荐值。启用后对重复的 system prompt 前缀加速，减少重复计算"
            />
          </template>
          <n-input-number
            v-model:value="formConfig.cache_reuse"
            :min="0"
            :step="1"
            placeholder="256"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>

        <!-- 空闲休眠 -->
        <n-form-item>
          <template #label>
            空闲休眠(秒)
            <HelpTip content="服务器空闲指定秒数后自动休眠以节省资源。-1=禁用休眠，0=立即休眠" />
          </template>
          <n-input-number
            v-model:value="formConfig.sleep_idle_seconds"
            :min="-1"
            :step="1"
            placeholder="-1"
            style="width: 100%"
            @blur="autoSave"
          />
        </n-form-item>

        <!-- 预填充助手消息 -->
        <n-form-item>
          <template #label>
            预填充助手消息
            <HelpTip
              content="开启后，最后一条助手消息会被预填充到 KV 缓存，加速后续推理。关闭则不预填充"
            />
          </template>
          <n-switch v-model:value="formConfig.prefill_assistant" />
        </n-form-item>
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
  NSwitch,
  NInput,
  NCollapseTransition,
  NText,
  NIcon
} from 'naive-ui'
import { ChevronDown, ChevronUp } from '@vicons/ionicons5'
import HelpTip from '../ui/HelpTip.vue'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'

defineOptions({ name: 'KvCacheSettings' })

// 从父级注入配置上下文（formConfig、autoSave 等共享状态）
const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)!
// C-5 域切片：KV 缓存类型选项归入 performance 域
const { core, performance } = ctx
const { formConfig, autoSave } = core
const { cacheTypeKOptions, cacheTypeVOptions } = performance

// 折叠状态：默认收起，避免一进设置页就看到一大片专家参数
const expanded = ref(false)

// 多 GPU 分割模式选项（对应 llama.cpp --split-mode）
const splitModeOptions = [
  { label: '按层分割 layer（默认）', value: 'layer' },
  { label: '按行分割 row', value: 'row' },
  { label: '按张量分割 tensor', value: 'tensor' },
  { label: '禁用多卡 none', value: 'none' }
]
</script>

<style scoped>
/* 高级设置折叠区域 */
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
  transition: transform 0.2s;
}

.advanced-content {
  padding: 12px 16px;
  background: var(--bg-tertiary);
}
</style>
