<!--
  PerformanceSettings: 性能与硬件设置组件（主容器）
  生活类比：像调汽车的驾驶模式——
    - 性能模式 = ECO/COMFORT/SPORT 驾驶模式
    - GPU 后端 = 发动机型号
    - KV 缓存 = 变速箱调校
    - 推测解码 = 涡轮增压器

  拆分后的组件结构：
    - BackendManager: GPU 硬件信息、后端切换与下载
    - PerformanceModeSelector: 性能模式选择（三档预设）
    - GpuAccelerationSettings: GPU 层数、Flash Attention
    - KvCacheSettings: KV 缓存、多 GPU 分割、线程等高级参数
    - SpeculativeDecodingSettings: 推测解码（draft/ngram）参数
-->
<template>
  <BackendManager />
  <PerformanceModeSelector />
  <GpuAccelerationSettings />
  <KvCacheSettings />
  <SpeculativeDecodingSettings />
</template>

<script setup lang="ts">
import BackendManager from './BackendManager.vue'
import PerformanceModeSelector from './PerformanceModeSelector.vue'
import GpuAccelerationSettings from './GpuAccelerationSettings.vue'
import KvCacheSettings from './KvCacheSettings.vue'
import SpeculativeDecodingSettings from './SpeculativeDecodingSettings.vue'

defineOptions({ name: 'PerformanceSettings' })
</script>
