<!--
  PerformanceSettings: 性能与硬件设置组件
  生活类比：像调汽车的驾驶模式——
    - 性能模式 = ECO/COMFORT/SPORT 驾驶模式
    - GPU 后端 = 发动机型号
    - KV 缓存 = 变速箱调校
    - 推测解码 = 涡轮增压器

  设计要点：
    - 性能模式卡片放在最前面（最直观，新手友好）
    - GPU 信息卡片展示当前硬件状态
    - 后端管理（选择/切换/下载）单独一组
    - KV 缓存设置折叠在"高级"子区（专家级参数）
    - 推测解码折叠在"加速"子区（专家级参数）
-->
<template>
  <!-- ==================== GPU 检测信息卡片 ==================== -->
  <div class="backend-gpu-info">
    <n-card class="gpu-info-card" hoverable>
      <div class="gpu-info-content">
        <n-icon size="20" class="gpu-info-icon"><HardwareChipOutline /></n-icon>
        <div class="gpu-info-text">
          <span class="gpu-info-label">GPU 厂商</span>
          <span class="gpu-info-value">{{ gpuVendorLabel }}</span>
        </div>
      </div>
    </n-card>

    <n-card class="gpu-info-card" hoverable>
      <div class="gpu-info-content">
        <n-icon size="20" class="gpu-info-icon"><SpeedometerOutline /></n-icon>
        <div class="gpu-info-text">
          <span class="gpu-info-label">GPU 名称</span>
          <span class="gpu-info-value">{{ backendStatus.gpu_name || '未检测到 GPU' }}</span>
        </div>
      </div>
    </n-card>

    <n-card class="gpu-info-card" hoverable>
      <div class="gpu-info-content">
        <n-icon size="20" class="gpu-info-icon"><ServerOutline /></n-icon>
        <div class="gpu-info-text">
          <span class="gpu-info-label">显存大小</span>
          <span class="gpu-info-value">{{ vramLabel }}</span>
        </div>
      </div>
    </n-card>
  </div>

  <!-- ==================== 当前后端状态 ==================== -->
  <n-form-item>
    <template #label>
      当前后端
      <HelpTip content="当前正在使用的计算后端（由启动时配置解析）。切换后端后需重启应用才能生效" />
    </template>
    <div class="backend-status-row">
      <n-tag :type="currentBackendTagType" size="small">{{ currentBackendLabel }}</n-tag>
      <span v-if="backendStatus.config_backend === 'auto'" class="backend-hint">
        （自动检测模式）
      </span>
      <span v-else class="backend-hint">
        （手动指定：{{ backendStatusLabel(backendStatus.config_backend) }}）
      </span>
    </div>
  </n-form-item>

  <!-- ==================== 性能模式选择器 ==================== -->
  <PerformanceModeSelector />

  <!-- ==================== 后端选择与切换 ==================== -->
  <n-form-item>
    <template #label>
      切换后端
      <HelpTip
        content="选择不同的计算后端。auto 会根据硬件自动选择最合适的后端。切换后需重启应用生效"
      />
    </template>
    <div class="backend-select-row">
      <n-select
        v-model:value="selectedBackend"
        :options="backendOptions"
        :loading="loading"
        :disabled="switching"
        placeholder="选择后端类型"
        class="backend-select"
      />
      <n-button
        type="primary"
        size="small"
        ghost
        :loading="switching"
        :disabled="!canApply"
        @click="handleApply"
      >
        应用
      </n-button>
    </div>
  </n-form-item>

  <!-- 后端安装状态列表 -->
  <n-form-item>
    <template #label>
      已安装后端
      <HelpTip
        content="runtime 目录中已存在 llama-server.exe 的后端。未安装的后端可点击下方下载按钮从 GitHub 获取"
      />
    </template>
    <div class="installed-backends">
      <n-tag
        v-for="bt in backendStatus.installed_backends"
        :key="bt"
        type="success"
        size="small"
        class="backend-tag"
      >
        {{ backendStatusLabel(bt) }}
      </n-tag>
      <span v-if="backendStatus.installed_backends.length === 0" class="empty-hint">
        尚无已安装后端
      </span>
    </div>
  </n-form-item>

  <!-- 下载缺失后端 -->
  <n-form-item>
    <template #label>
      下载后端
      <HelpTip
        content="从 GitHub llama.cpp releases 下载缺失的后端 zip 包，下载后自动解压安装。需重启应用生效"
      />
    </template>
    <div class="download-row">
      <n-select
        v-model:value="selectedDownloadBackend"
        :options="downloadOptions"
        :disabled="downloading"
        placeholder="选择要下载的后端"
        class="download-select"
      />
      <n-button
        type="primary"
        size="small"
        ghost
        :loading="downloading"
        :disabled="!canDownload"
        @click="handleDownload"
      >
        下载
      </n-button>
    </div>
  </n-form-item>

  <!-- ==================== GPU 加速设置 ==================== -->
  <n-divider style="margin: 24px 0 16px" />
  <div class="section-header">
    <span class="section-icon">🚀</span>
    <span class="section-title">GPU 加速</span>
  </div>

  <!-- 模型量化类型（只读显示） -->
  <n-form-item v-if="modelFtype">
    <template #label>
      量化类型
      <HelpTip content="当前模型的量化格式，从 GGUF 元数据解析。影响模型质量与显存占用的平衡" />
    </template>
    <n-tag size="small" type="info">{{ modelFtype }}</n-tag>
  </n-form-item>

  <!-- GPU 状态 -->
  <n-form-item>
    <template #label>
      GPU 状态
      <HelpTip
        content="自动检测 NVIDIA GPU。显示'已检测'表示有 N 卡驱动但无法获取型号信息，GPU 参数仍会自动配置为全卸载模式"
      />
    </template>
    <div class="gpu-status-row">
      <n-tag v-if="gpuInfo.has_gpu" type="success" size="small">
        {{ gpuInfo.gpu_name }} ({{ gpuInfo.vram_gb }}GB)
      </n-tag>
      <n-tag v-else-if="gpuInfo.has_cuda_backend" type="warning" size="small">
        CUDA 驱动已检测
      </n-tag>
      <n-tag v-else type="error" size="small">未检测到 NVIDIA GPU</n-tag>
    </div>
  </n-form-item>

  <!-- GPU 层数 -->
  <n-form-item>
    <template #label>
      GPU 层数
      <HelpTip
        content="加载到 GPU 的模型层数。0=自动（有 N 卡时全部卸载），99=全部卸载到 GPU，1=仅第一层在 GPU。检测到 N 卡但显示 CPU 运行时，手动设为 99 可强制使用 GPU"
      />
    </template>
    <n-input-number
      v-model:value="formConfig.gpu_layers"
      :min="0"
      :max="99"
      :step="1"
      placeholder="0 = 自动"
      style="width: 100%"
      @blur="autoSave"
    />
  </n-form-item>

  <!-- Flash Attention -->
  <n-form-item>
    <template #label>
      Flash Attention
      <HelpTip
        content="GPU 上的注意力加速技术，大幅提升推理速度并降低显存占用。自动=有 GPU 时开启，无 GPU 时关闭。CPU 模式下强制开启会报错"
      />
    </template>
    <n-select v-model:value="flashAttnValue" :options="flashAttnOptions" @update:value="autoSave" />
  </n-form-item>

  <!-- ==================== KV 缓存设置（折叠） ==================== -->
  <div class="advanced-section">
    <div class="advanced-header" @click="kvCacheExpanded = !kvCacheExpanded">
      <span class="advanced-icon">💾</span>
      <span class="advanced-title">KV 缓存</span>
      <n-icon class="advanced-toggle" :component="kvCacheExpanded ? ChevronUp : ChevronDown" />
    </div>
    <n-collapse-transition>
      <div v-if="kvCacheExpanded" class="advanced-content">
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
              content="开启后允许将 KV 缓存卸载到 CPU 内存，节省显存但会降低速度。显存不足时可开启"
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

  <!-- ==================== 推测解码（折叠） ==================== -->
  <div class="advanced-section">
    <div class="advanced-header" @click="speculativeExpanded = !speculativeExpanded">
      <span class="advanced-icon">⚡</span>
      <span class="advanced-title">推测解码（加速推理）</span>
      <n-icon class="advanced-toggle" :component="speculativeExpanded ? ChevronUp : ChevronDown" />
    </div>
    <n-collapse-transition>
      <div v-if="speculativeExpanded" class="advanced-content">
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
              content="加速推理的推测解码技术。draft-mtp 需要模型内置 MTP 头（如 Qwen3.6-UD），draft-eagle3 需要 Eagle3 草稿模型，ngram 类型对所有模型可用。自动模式下检测到 MTP 头会自动启用 draft-mtp，否则启用 ngram-mod"
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
              formConfig.spec_type === 'draft-simple')
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
                content="Eagle3/DFlash 或 Draft 草稿模型的 .gguf 文件路径。draft-eagle3/draft-dflash/draft-simple 模式需要"
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

  <!-- ==================== 下载进度对话框 ==================== -->
  <n-modal
    v-model:show="showProgressDialog"
    :mask-closable="false"
    :close-on-esc="false"
    preset="card"
    :title="progressDialogTitle"
    style="width: 480px; max-width: 90vw"
  >
    <div class="progress-content">
      <n-progress
        type="line"
        :percentage="Math.round(downloadProgress.Percent)"
        :status="progressStatus"
        :indicator-placement="'inside'"
        processing
      />
      <div class="progress-info">
        <span v-if="downloadProgress.Status === 'downloading'" class="progress-detail">
          {{ formatBytes(downloadProgress.Downloaded) }} /
          {{ formatBytes(downloadProgress.TotalBytes) }} （{{ downloadProgress.AssetName }}）
        </span>
        <span v-else-if="downloadProgress.Status === 'installing'" class="progress-detail">
          正在解压安装...
        </span>
        <span
          v-else-if="downloadProgress.Status === 'completed'"
          class="progress-detail success-text"
        >
          下载完成！
        </span>
        <span v-else-if="downloadProgress.Status === 'failed'" class="progress-detail error-text">
          下载失败：{{ downloadProgress.Error }}
        </span>
      </div>
      <div v-if="downloadProgress.Status === 'completed'" class="progress-hint">
        后端已下载并安装完成，请重启应用使其生效。
      </div>
    </div>
    <template #footer>
      <div class="progress-footer">
        <n-button
          v-if="downloadProgress.Status === 'completed' || downloadProgress.Status === 'failed'"
          size="small"
          @click="closeProgressDialog"
        >
          关闭
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { inject, ref, computed, onMounted, onUnmounted, watch } from 'vue'
import {
  NFormItem,
  NSelect,
  NButton,
  NTag,
  NCard,
  NIcon,
  NModal,
  NProgress,
  NInputNumber,
  NSwitch,
  NInput,
  NDivider,
  NCollapseTransition,
  NText,
  useDialog,
  useMessage
} from 'naive-ui'
import { HardwareChipOutline, SpeedometerOutline, ServerOutline } from '@vicons/ionicons5'
import { ChevronDown, ChevronUp } from '@vicons/ionicons5'
import {
  wails,
  type BackendStatus,
  type BackendDownloadProgress,
  type BackendDownloadComplete
} from '../../services/wails'
import { logError } from '../../utils/logger'
import HelpTip from '../ui/HelpTip.vue'
import PerformanceModeSelector from './PerformanceModeSelector.vue'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import { useSettingsStore } from '../../stores/settings'

defineOptions({ name: 'PerformanceSettings' })

// ===== 从 settingsContext 注入配置 =====
const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)!
const { formConfig, autoSave, cacheTypeKOptions, cacheTypeVOptions, specTypeOptions } = ctx
const settingsStore = useSettingsStore()
const dialog = useDialog()
const message = useMessage()

// ===== 后端状态（从后端 GetBackendStatus 拉取） =====
const backendStatus = ref<BackendStatus>({
  current_backend: '',
  config_backend: 'auto',
  gpu_vendor: '',
  gpu_name: '',
  gpu_vram_mb: 0,
  installed_backends: [],
  available_backends: []
})

// 用户在下拉框中选择的值（独立于 backendStatus.config_backend，点击"应用"才提交）
const selectedBackend = ref<string>('auto')
const loading = ref(false)
const switching = ref(false)

// ===== 下载后端相关状态 =====
const selectedDownloadBackend = ref<string>('')
const downloading = ref(false)
const showProgressDialog = ref(false)
const downloadProgress = ref<BackendDownloadProgress>({
  Backend: '',
  AssetName: '',
  TagName: '',
  TotalBytes: 0,
  Downloaded: 0,
  Percent: 0,
  Status: '',
  Error: '',
  Label: ''
})

// ===== 模型量化类型（从 GGUF 元数据解析） =====
const modelFtype = ref('')

// ===== GPU 状态信息 =====
const gpuInfo = ref({ has_gpu: false, has_cuda_backend: false, gpu_name: '', vram_gb: 0 })

// ===== Flash Attention 三态选项 =====
const flashAttnOptions = [
  { label: '自动（有 GPU 时开启）', value: 'auto' },
  { label: '开启', value: 'on' },
  { label: '关闭', value: 'off' }
]
const flashAttnValue = computed({
  get: () => {
    if (formConfig.value.flash_attn === null) return 'auto'
    return formConfig.value.flash_attn ? 'on' : 'off'
  },
  set: (val: string) => {
    if (val === 'auto') formConfig.value.flash_attn = null
    else formConfig.value.flash_attn = val === 'on'
  }
})

// ===== 折叠状态 =====
const kvCacheExpanded = ref(false)
const speculativeExpanded = ref(false)

// ===== 后端类型 -> 中文显示名映射 =====
const backendDisplayNames: Record<string, string> = {
  auto: '自动检测（推荐）',
  cuda: 'CUDA (NVIDIA)',
  hip: 'HIP (AMD)',
  sycl: 'SYCL (Intel)',
  vulkan: 'Vulkan (跨厂商)',
  openvino: 'OpenVINO (Intel)',
  cpu: 'CPU (纯 CPU)'
}

const backendDescriptions: Record<string, string> = {
  auto: '自动检测',
  cuda: 'CUDA',
  hip: 'HIP',
  sycl: 'SYCL',
  vulkan: 'Vulkan',
  openvino: 'OpenVINO',
  cpu: 'CPU'
}

/** 将后端类型转为中文显示名 */
function backendStatusLabel(bt: string): string {
  return backendDisplayNames[bt] || backendDescriptions[bt] || bt
}

/** 下拉框选项：基于 available_backends 生成 */
const backendOptions = computed(() => {
  return backendStatus.value.available_backends.map(bt => ({
    label: backendStatusLabel(bt),
    value: bt
  }))
})

/** GPU 厂商中文显示 */
const gpuVendorLabel = computed(() => {
  const vendor = backendStatus.value.gpu_vendor
  if (!vendor) return '未检测到'
  const vendorMap: Record<string, string> = {
    nvidia: 'NVIDIA',
    amd: 'AMD',
    intel: 'Intel',
    vulkan: 'Vulkan'
  }
  return vendorMap[vendor] || vendor
})

/** 显存大小显示（自动转换为 GB） */
const vramLabel = computed(() => {
  const vram = backendStatus.value.gpu_vram_mb
  if (!vram) return '无'
  if (vram >= 1024) {
    return `${(vram / 1024).toFixed(1)} GB`
  }
  return `${vram} MB`
})

/** 当前后端显示标签 */
const currentBackendLabel = computed(() => {
  const cur = backendStatus.value.current_backend
  if (!cur || cur === 'auto') return '自动检测'
  return backendStatusLabel(cur)
})

/** 当前后端标签颜色 */
const currentBackendTagType = computed<'success' | 'warning' | 'info'>(() => {
  const cur = backendStatus.value.current_backend
  if (cur === 'cpu') return 'warning'
  if (cur === 'auto' || !cur) return 'info'
  return 'success'
})

/** 是否可以点击应用：选择值与当前配置值不同时才可应用 */
const canApply = computed(() => {
  return selectedBackend.value !== backendStatus.value.config_backend && !switching.value
})

/** 下载选项：排除 auto，仅显示具体后端 */
const downloadOptions = computed(() => {
  const allBackends = backendStatus.value.available_backends.filter(bt => bt !== 'auto')
  return allBackends.map(bt => ({
    label: backendStatusLabel(bt),
    value: bt
  }))
})

/** 是否可以点击下载：选择了后端且不在下载中 */
const canDownload = computed(() => {
  return selectedDownloadBackend.value !== '' && !downloading.value
})

/** 进度对话框标题 */
const progressDialogTitle = computed(() => {
  const bt = downloadProgress.value.Backend
  return bt ? `下载 ${backendStatusLabel(bt)} 后端` : '下载后端'
})

/** n-progress 状态 */
const progressStatus = computed<'success' | 'error' | 'default'>(() => {
  const s = downloadProgress.value.Status
  if (s === 'completed') return 'success'
  if (s === 'failed') return 'error'
  return 'default'
})

/** 格式化字节数为可读字符串 */
function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

/** 处理下载按钮点击 */
function handleDownload() {
  const target = selectedDownloadBackend.value
  if (!target) return

  dialog.warning({
    title: '下载显卡后端',
    content: `即将从 GitHub 下载 ${backendStatusLabel(target)} 后端 zip 包，\n下载完成后自动解压安装。\n\n是否继续？`,
    positiveText: '确认下载',
    negativeText: '取消',
    onPositiveClick: () => {
      doDownloadBackend(target)
    }
  })
}

/** 执行后端下载 */
async function doDownloadBackend(target: string) {
  downloading.value = true
  showProgressDialog.value = true
  downloadProgress.value = {
    Backend: target,
    AssetName: '',
    TagName: '',
    TotalBytes: 0,
    Downloaded: 0,
    Percent: 0,
    Status: 'downloading',
    Error: '',
    Label: ''
  }
  try {
    await wails.downloadBackend(target)
  } catch (e) {
    logError('Failed to start backend download', e)
    message.error(`下载启动失败：${e instanceof Error ? e.message : String(e)}`)
    downloading.value = false
    showProgressDialog.value = false
  }
}

/** 关闭进度对话框 */
function closeProgressDialog() {
  showProgressDialog.value = false
  downloading.value = false
  loadBackendStatus()
}

/** 加载后端状态 */
async function loadBackendStatus() {
  loading.value = true
  try {
    const status = await wails.getBackendStatus()
    backendStatus.value = status
    selectedBackend.value = status.config_backend || 'auto'
  } catch (e) {
    logError('Failed to load backend status', e)
    message.error('加载后端状态失败')
  } finally {
    loading.value = false
  }
}

/** 处理应用按钮点击 */
function handleApply() {
  const target = selectedBackend.value
  if (target === backendStatus.value.config_backend) return

  dialog.warning({
    title: '切换显卡后端',
    content: `切换后端需要重启应用才能生效。\n\n即将切换到：${backendStatusLabel(target)}\n\n是否继续？`,
    positiveText: '确认切换',
    negativeText: '取消',
    onPositiveClick: () => {
      doSwitchBackend(target)
    }
  })
}

/** 执行后端切换 */
async function doSwitchBackend(target: string) {
  switching.value = true
  message.loading('正在切换后端...', { duration: 0 })
  try {
    await wails.switchBackend(target)
    message.destroyAll()
    message.success('后端配置已保存，重启应用后生效', { duration: 5000 })
    await loadBackendStatus()
  } catch (e) {
    message.destroyAll()
    logError('Failed to switch backend', e)
    const errMsg = e instanceof Error ? e.message : String(e)
    message.error(`后端切换失败：${errMsg}`, { duration: 5000 })
    await loadBackendStatus()
  } finally {
    switching.value = false
  }
}

/** 加载模型量化类型 */
async function loadModelFtype() {
  try {
    const smartParams = await wails.getSmartParams()
    modelFtype.value = smartParams.model.ftype || ''
    gpuInfo.value = {
      has_gpu: smartParams.hardware.has_gpu,
      has_cuda_backend: smartParams.hardware.has_cuda_backend,
      gpu_name: smartParams.hardware.gpu_name,
      vram_gb: Math.round(smartParams.hardware.gpu_vram_mb / 1024)
    }
  } catch {
    modelFtype.value = ''
  }
}

// ===== 事件监听 =====
let unsubscribeBackendSwitched: (() => void) | null = null
let unsubscribeDownloadProgress: (() => void) | null = null
let unsubscribeDownloadComplete: (() => void) | null = null

onMounted(() => {
  loadBackendStatus()
  loadModelFtype()
  unsubscribeBackendSwitched = wails.subscribeBackendSwitched((status: BackendStatus) => {
    backendStatus.value = status
    selectedBackend.value = status.config_backend || 'auto'
  })
  unsubscribeDownloadProgress = wails.subscribeBackendDownloadProgress(
    (progress: BackendDownloadProgress) => {
      downloadProgress.value = progress
      if (progress.Status === 'completed' || progress.Status === 'failed') {
        downloading.value = false
      }
    }
  )
  unsubscribeDownloadComplete = wails.subscribeBackendDownloadComplete(
    (result: BackendDownloadComplete) => {
      if (result.success) {
        downloadProgress.value = {
          ...downloadProgress.value,
          Backend: result.backend,
          Status: 'completed',
          Percent: 100,
          Label: '下载完成'
        }
        downloading.value = false
        message.success(`${backendStatusLabel(result.backend)} 后端下载安装完成，请重启应用生效`, {
          duration: 5000
        })
      } else {
        downloadProgress.value = {
          ...downloadProgress.value,
          Backend: result.backend,
          Status: 'failed',
          Error: result.error || '未知错误'
        }
        downloading.value = false
        message.error(
          `${backendStatusLabel(result.backend)} 后端安装失败：${result.error || '未知错误'}`,
          { duration: 8000 }
        )
      }
      loadBackendStatus()
    }
  )
})

onUnmounted(() => {
  if (unsubscribeBackendSwitched) {
    unsubscribeBackendSwitched()
    unsubscribeBackendSwitched = null
  }
  if (unsubscribeDownloadProgress) {
    unsubscribeDownloadProgress()
    unsubscribeDownloadProgress = null
  }
  if (unsubscribeDownloadComplete) {
    unsubscribeDownloadComplete()
    unsubscribeDownloadComplete = null
  }
})

// 模型切换时重新加载量化类型
watch(() => settingsStore.currentModel, loadModelFtype)
</script>

<style scoped>
/* GPU 信息卡片网格布局 */
.backend-gpu-info {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 10px;
  margin-bottom: 16px;
}

.gpu-info-card {
  cursor: default;
  transition:
    transform 0.15s ease,
    box-shadow 0.15s ease;
}

.gpu-info-card:hover {
  transform: translateY(-1px);
}

.gpu-info-card :deep(.n-card__content) {
  padding: 12px 14px;
}

.gpu-info-content {
  display: flex;
  align-items: center;
  gap: 10px;
}

.gpu-info-icon {
  color: var(--text-secondary);
  flex-shrink: 0;
}

.gpu-info-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.gpu-info-label {
  font-size: 11px;
  color: var(--text-muted);
}

.gpu-info-value {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 后端状态行 */
.backend-status-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.backend-hint {
  font-size: 12px;
  color: var(--text-muted);
}

/* 后端选择行 */
.backend-select-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.backend-select {
  flex: 1;
  max-width: 320px;
}

/* 已安装后端标签列表 */
.installed-backends {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.backend-tag {
  margin: 0;
}

.empty-hint {
  font-size: 12px;
  color: var(--text-muted);
}

/* 下载后端选择行 */
.download-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.download-select {
  flex: 1;
  max-width: 320px;
}

/* 响应式：窄屏单列 */
@media (max-width: 480px) {
  .backend-gpu-info {
    grid-template-columns: 1fr;
  }
}

/* 下载进度对话框 */
.progress-content {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.progress-info {
  font-size: 13px;
  color: var(--text-secondary);
}

.progress-detail {
  word-break: break-all;
}

.progress-detail.success-text {
  color: var(--n-color-success, #18a058);
}

.progress-detail.error-text {
  color: var(--n-color-error, #d03050);
}

.progress-hint {
  font-size: 12px;
  color: var(--text-muted);
  padding: 8px 12px;
  background: var(--n-color-target, rgba(255, 255, 255, 0.06));
  border-radius: 4px;
}

.progress-footer {
  display: flex;
  justify-content: flex-end;
}

/* GPU 状态行 */
.gpu-status-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 区块标题 */
.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}

.section-icon {
  font-size: 18px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
}

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
