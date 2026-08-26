<!--
  PerformanceSettings: 性能与硬件设置组件（主容器）

  拆分后的组件结构：
    - BackendManager: GPU 硬件信息、后端切换与下载
    - GpuAccelerationSettings: GPU 层数、Flash Attention
    - KvCacheSettings: KV 缓存、多 GPU 分割、线程等高级参数
    - SpeculativeDecodingSettings: 推测解码（draft/ngram）参数
-->
<template>
  <BackendManager />
  <GpuAccelerationSettings />
  <KvCacheSettings />
  <SpeculativeDecodingSettings />
</template>

<script setup lang="ts">
import { defineAsyncComponent } from 'vue'
// 性能项：后端管理器含下载逻辑且非首屏必需，异步加载
const BackendManager = defineAsyncComponent(() => import('./BackendManager.vue'))
import GpuAccelerationSettings from './GpuAccelerationSettings.vue'
import KvCacheSettings from './KvCacheSettings.vue'
import SpeculativeDecodingSettings from './SpeculativeDecodingSettings.vue'

defineOptions({ name: 'PerformanceSettings' })
</script>
