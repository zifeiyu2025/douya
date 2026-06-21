<template>
  <div class="settings-container">
    <div class="settings-header">
      <n-button quaternary circle @click="$router.push('/')">
        <template #icon>
          <n-icon><ArrowBackOutline /></n-icon>
        </template>
      </n-button>
      <span class="settings-title">设置</span>
    </div>
    <div class="settings-content">
      <n-form label-placement="left" label-width="120" :model="formConfig">
        <n-divider>外观设置</n-divider>

        <n-form-item label="聊天背景">
          <div class="upload-wrapper">
              <div class="upload-placeholder" v-if="!formConfig.chat_background" @click="selectBackgroundImage">
                <div class="upload-icon">🖼️</div>
                <div class="upload-text">点击选择背景图片</div>
              </div>
              <div class="upload-preview" v-else @click="selectBackgroundImage">
                <img :src="backgroundImageUrl" class="background-preview" />
                <div class="hover-overlay">
                  <span class="hover-hint">点击更改</span>
                </div>
                <div class="upload-actions">
                  <n-button size="small" text @click.stop="clearBackground">清除</n-button>
                </div>
              </div>
          </div>
        </n-form-item>

        <n-form-item label="背景透明度" v-if="formConfig.chat_background">
          <n-slider v-model:value="formConfig.chat_background_opacity" :min="0.2" :max="1" :step="0.05" />
          <span class="slider-value">{{ Math.round(formConfig.chat_background_opacity * 100) }}%</span>
        </n-form-item>

        <n-form-item label="用户头像">
          <div class="avatar-upload-wrapper">
            <div class="avatar-preview">
              <img :src="formConfig.user_avatar || defaultUserAvatar" class="avatar-img" />
            </div>
            <div class="avatar-buttons">
              <n-upload
                :show-file-list="false"
                :custom-request="handleUserAvatarUpload"
                accept="image/*"
              >
                <n-button size="small" quaternary>上传</n-button>
              </n-upload>
              <n-button size="small" text @click="clearUserAvatar" v-if="formConfig.user_avatar">清除</n-button>
            </div>
          </div>
        </n-form-item>

        <n-form-item label="AI头像">
          <div class="avatar-upload-wrapper">
            <div class="avatar-preview ai-avatar">
              <img :src="formConfig.ai_avatar || defaultAiAvatar" class="avatar-img" />
            </div>
            <div class="avatar-buttons">
              <n-upload
                :show-file-list="false"
                :custom-request="handleAIAvatarUpload"
                accept="image/*"
              >
                <n-button size="small" quaternary>上传</n-button>
              </n-upload>
              <n-button size="small" text @click="clearAIAvatar" v-if="formConfig.ai_avatar">清除</n-button>
            </div>
          </div>
        </n-form-item>

        <n-divider>搜索 API KEY</n-divider>

        <n-form-item label="Ollama API Key">
          <div class="api-key-row">
            <n-input
              v-model:value="newOllamaApiKey"
              type="password"
              show-password-on="click"
              :placeholder="searchKeys.ollama_api_key_set ? '已设置，输入新值覆盖' : '输入 API Key 保存'"
              @blur="saveSearchKeys"
              class="api-key-input"
            />
            <n-tag v-if="searchKeys.ollama_api_key_set" type="success" size="small">已设置</n-tag>
          </div>
          <template #feedback>
            <span class="api-key-hint">
              获取地址：<n-a href="https://ollama.com/settings/keys" target="_blank">https://ollama.com/settings/keys</n-a>
            </span>
          </template>
        </n-form-item>

        <n-form-item label="Tavily API Key">
          <div class="api-key-row">
            <n-input
              v-model:value="newTavilyApiKey"
              type="password"
              show-password-on="click"
              :placeholder="searchKeys.tavily_api_key_set ? '已设置，输入新值覆盖' : '输入 API Key 保存'"
              @blur="saveSearchKeys"
              class="api-key-input"
            />
            <n-tag v-if="searchKeys.tavily_api_key_set" type="success" size="small">已设置</n-tag>
          </div>
          <template #feedback>
            <span class="api-key-hint">
              获取地址：<n-a href="https://app.tavily.com/" target="_blank">https://app.tavily.com/</n-a>
              （免费额度 1000 次/月）
            </span>
          </template>
        </n-form-item>

        <n-divider>服务 API KEY</n-divider>
        <n-form-item>
          <template #label>本机服务地址 <HelpTip content="llama-server 的 HTTP 服务地址。默认 http://127.0.0.1:8080，修改后需重启应用生效" /></template>
          <n-input
            v-model:value="formConfig.api_base"
            placeholder="http://127.0.0.1:8080"
            @blur="autoSave"
          />
        </n-form-item>
        <n-form-item label="暴露服务器地址" label-width="140" label-placement="left">
          <n-switch v-model:value="formConfig.expose_server" @update:value="onExposeServerToggle" />
          <span class="setting-hint">开启后局域网设备可通过本机 IP 访问（需重启服务生效）</span>
        </n-form-item>
        <n-form-item label-width="140" label-placement="left">
          <template #label>启用 API Key 验证 <HelpTip content="开启后所有 API 请求需携带 API Key，防止未授权访问。暴露到局域网时强烈建议开启" /></template>
          <n-switch v-model:value="formConfig.server_api_key_enabled" @update:value="onServerAPIKeyToggle" />
        </n-form-item>
        <n-form-item v-if="formConfig.server_api_key_enabled" label="API Key" label-width="80">
          <div class="api-key-row">
            <n-input
              v-model:value="serverApiKey"
              type="password"
              show-password-on="click"
              :placeholder="hasServerApiKey ? '已设置，留空保持不变' : '设置后 API 请求需携带此密钥'"
              @blur="saveServerApiKey"
              class="api-key-input"
            />
            <n-tag v-if="hasServerApiKey" type="success" size="small">已设置</n-tag>
          </div>
        </n-form-item>

        <n-divider>系统提示词</n-divider>
        <n-form-item>
          <template #label>系统提示词 <HelpTip content="追加在豆芽默认提示词后面的自定义指令，用于补充角色设定和行为约束。留空则仅使用默认提示词" /></template>
          <n-input v-model:value="formConfig.system_prompt" type="textarea" placeholder="自定义提示词将追加在豆芽默认提示词后面，用于补充角色设定和行为指令..." :autosize="{ minRows: 6, maxRows: 20 }" class="rounded-textarea" style="width: 100%;" />
        </n-form-item>

        <n-divider>推理配置</n-divider>
        <n-form-item>
          <template #label>推理模式 <HelpTip content="控制模型的推理（思考）行为。on=始终开启推理，off=关闭推理，auto=由模型自行决定是否推理" /></template>
          <n-select v-model:value="formConfig.reasoning" :options="reasoningOptions" :disabled="!supportsReasoning" />
        </n-form-item>
        <n-text v-if="!supportsReasoning" depth="3" style="font-size: 12px; margin-top: -12px; display: block; margin-bottom: 8px;">
          当前模型不支持推理
        </n-text>
        <n-form-item>
          <template #label>推理预算 <HelpTip content="限制推理（思考）过程的 token 数量。-1=无限（由模型自行决定），0=立即结束推理，N>0=限制为 N 个 token" /></template>
          <n-input-number v-model:value="formConfig.reasoning_budget" :min="-1" :step="1" placeholder="-1" style="width: 100%" :disabled="!supportsReasoning" />
        </n-form-item>
        <n-form-item v-if="formConfig.reasoning_budget > 0">
          <template #label>预算耗尽消息 <HelpTip content="推理预算耗尽时显示给用户的消息。留空则使用默认提示" /></template>
          <n-input v-model:value="formConfig.reasoning_budget_message" placeholder="推理预算耗尽时显示给用户的消息（留空使用默认提示）" @blur="autoSave" :disabled="!supportsReasoning" />
        </n-form-item>

        <n-divider>生成参数</n-divider>

        <div v-if="currentModelRef" class="model-ref-card">
          <div class="model-ref-header">
            <span class="model-ref-icon">📋</span>
            <span class="model-ref-title">{{ currentModelRef.name }} 官方参考参数</span>
            <span class="model-ref-current" v-if="settingsStore.currentModel">当前: {{ settingsStore.currentModel }}</span>
          </div>
          <div v-if="currentModelRef.raw_thinking" class="model-ref-tabs">
            <button
              class="model-ref-tab"
              :class="{ active: !refShowThinking }"
              @click="refShowThinking = false"
            >非思考模式</button>
            <button
              class="model-ref-tab"
              :class="{ active: refShowThinking }"
              @click="refShowThinking = true"
            >思考模式</button>
          </div>
          <div class="model-ref-body">
            <template v-if="!refShowThinking">
              <div class="model-ref-row" v-for="item in currentModelRef.params" :key="item.label">
                <span class="model-ref-label">{{ item.label }}</span>
                <span class="model-ref-value">{{ item.value }}</span>
              </div>
            </template>
            <template v-else-if="currentModelRef.params_thinking">
              <div class="model-ref-row" v-for="item in currentModelRef.params_thinking" :key="item.label">
                <span class="model-ref-label">{{ item.label }}</span>
                <span class="model-ref-value">{{ item.value }}</span>
              </div>
            </template>
            <div v-if="currentModelRef.note" class="model-ref-note">{{ currentModelRef.note }}</div>
          </div>
          <n-button size="tiny" quaternary class="model-ref-apply" @click="applyModelRef">
            应用参考参数
          </n-button>
        </div>

        <n-form-item>
          <template #label>温度 <HelpTip content="控制回答的随机性。值越低越确定保守，值越高越多样创意。一般 0.3-0.8 之间" /></template>
          <n-slider v-model:value="formConfig.temperature" :min="0" :max="2" :step="0.01" />
          <span class="slider-value">{{ formConfig.temperature }}</span>
          <n-button v-if="currentModelRef" quaternary size="tiny" class="reset-btn" @click="formConfig.temperature = activeModelRefRaw.temperature">{{ activeModelRefRaw.temperature }}</n-button>
        </n-form-item>
        <n-form-item>
          <template #label>Top P <HelpTip content="从概率最高的候选词中筛选，只考虑累计概率达到此阈值的词。0.95 表示保留前 95% 概率的词" /></template>
          <n-slider v-model:value="formConfig.top_p" :min="0" :max="1" :step="0.01" />
          <span class="slider-value">{{ formConfig.top_p }}</span>
          <n-button v-if="currentModelRef" quaternary size="tiny" class="reset-btn" @click="formConfig.top_p = activeModelRefRaw.top_p">{{ activeModelRefRaw.top_p }}</n-button>
        </n-form-item>
        <n-form-item>
          <template #label>Top K <HelpTip content="只从概率最高的 K 个候选词中选择。值越小选择越少越确定，0 表示不限制" /></template>
          <n-slider v-model:value="formConfig.top_k" :min="0" :max="100" :step="1" />
          <span class="slider-value">{{ formConfig.top_k }}</span>
          <n-button v-if="currentModelRef" quaternary size="tiny" class="reset-btn" @click="formConfig.top_k = activeModelRefRaw.top_k">{{ activeModelRefRaw.top_k }}</n-button>
        </n-form-item>
        <n-form-item>
          <template #label>上下文长度 <HelpTip content="模型能记住的对话历史 token 数。值越大记忆越长但占用显存越多，超过模型支持的最大值会被自动截断" /></template>
          <n-slider v-model:value="contextSizeIndex" :min="0" :max="contextSizeSteps.length - 1" :step="1" :marks="contextSizeMarks" />
          <span class="slider-value">{{ formatContextSize(formConfig.context_size) }}</span>
          <n-button v-if="currentModelRef" quaternary size="tiny" class="reset-btn" @click="applyContextSizeRef">{{ formatContextSize(activeModelRefRaw.context_size) }}</n-button>
        </n-form-item>
        <n-form-item>
          <template #label>重复惩罚 <HelpTip content="大于 1 时惩罚重复内容，防止 AI 反复说同样的话。1.0 表示不惩罚" /></template>
          <n-slider v-model:value="formConfig.repeat_penalty" :min="0" :max="2" :step="0.01" />
          <span class="slider-value">{{ formConfig.repeat_penalty }}</span>
        </n-form-item>
        <n-form-item>
          <template #label>Min-P <HelpTip content="根据最高概率词动态过滤低概率词。0.05 表示过滤掉概率不到最高词 5% 的候选词" /></template>
          <n-slider v-model:value="formConfig.min_p" :min="0" :max="1" :step="0.01" />
          <span class="slider-value">{{ formConfig.min_p }}</span>
        </n-form-item>
        <n-form-item>
          <template #label>DRY 采样倍数 <HelpTip content="防止 AI 重复相同句式。0 表示关闭，大于 0 时值越强越不容易重复" /></template>
          <n-slider v-model:value="formConfig.dry_multiplier" :min="0" :max="5" :step="0.01" />
          <span class="slider-value">{{ formConfig.dry_multiplier }}</span>
        </n-form-item>
        <n-form-item v-if="formConfig.dry_multiplier > 0">
          <template #label>DRY 基准值 <HelpTip content="DRY 采样的基础惩罚倍数。值越大对重复句式的惩罚越强，通常 1.0-2.0" /></template>
          <n-slider v-model:value="formConfig.dry_base" :min="1" :max="3" :step="0.01" />
          <span class="slider-value">{{ formConfig.dry_base }}</span>
        </n-form-item>
        <n-form-item v-if="formConfig.dry_multiplier > 0">
          <template #label>DRY 允许长度 <HelpTip content="允许重复的 token 序列长度。短于此长度的重复不会被惩罚，值越小越严格" /></template>
          <n-slider v-model:value="formConfig.dry_allowed_length" :min="1" :max="10" :step="1" />
          <span class="slider-value">{{ formConfig.dry_allowed_length }}</span>
        </n-form-item>
        <n-divider style="margin: 8px 0" />
        <n-form-item>
          <template #label>内存映射 (mmap) <HelpTip content="将模型文件映射到内存而非全部加载。开启可加快启动速度、减少内存占用，关闭则全部预加载到内存" /></template>
          <n-switch v-model:value="formConfig.mmap" />
        </n-form-item>
        <n-form-item>
          <template #label>KV 缓存 K 类型 <HelpTip content="Key 缓存的量化精度。K 决定注意力查找方向，建议用高精度（q8_0）。选「自动」由系统根据显存自动选择" /></template>
          <n-select v-model:value="formConfig.cache_type_k" :options="cacheTypeKOptions" placeholder="自动（q8_0）" clearable />
        </n-form-item>
        <n-form-item>
          <template #label>KV 缓存 V 类型 <HelpTip content="Value 缓存的量化精度。V 是实际内容，可以更激进压缩。选「自动」由系统智能选择" /></template>
          <n-select v-model:value="formConfig.cache_type_v" :options="cacheTypeVOptions" placeholder="自动（q4_0）" clearable />
        </n-form-item>
        <n-form-item>
          <template #label>KV 缓存卸载 <HelpTip content="开启后允许将 KV 缓存卸载到 CPU 内存，节省显存但会降低速度。显存不足时可开启" /></template>
          <n-switch v-model:value="formConfig.kv_offload" />
        </n-form-item>
        <n-form-item>
          <template #label>上下文移位 <HelpTip content="当对话超出上下文长度时，自动移除最早的内容腾出空间，而非直接报错。开启可支持更长的连续对话" /></template>
          <n-switch v-model:value="formConfig.context_shift" />
        </n-form-item>
        <n-form-item>
          <template #label>GPU 设备 <HelpTip content="指定使用的 GPU 设备。留空自动选择，多卡场景用逗号分隔（如 0,1）" /></template>
          <n-input v-model:value="formConfig.device" placeholder="留空自动选择，多卡如 0,1" />
        </n-form-item>
        <n-form-item>
          <template #label>并发槽位数 <HelpTip content="同时处理的请求数。0=自动（通常为 1），增大可支持多用户并发但会按比例增加显存占用" /></template>
          <n-input-number v-model:value="formConfig.parallel" :min="0" placeholder="0 = 自动" style="width: 100%" />
        </n-form-item>

        <div class="gen-params-save-row">
          <span class="gen-params-status" v-if="genParamsDirty">设置已修改，自动保存中...</span>
          <span class="gen-params-status saved" v-else-if="formConfig.context_size > 0">✓ 已保存</span>
        </div>

        <n-divider>推测解码</n-divider>
        <n-form-item>
          <template #label>推测类型 <HelpTip content="加速推理的推测解码技术。draft-mtp 需要模型内置 MTP 头（如 Qwen3.6-UD），draft-eagle3 需要 Eagle3 草稿模型，ngram 类型对所有模型可用。自动模式下检测到 MTP 头会自动启用 draft-mtp，否则启用 ngram-mod" /></template>
          <n-select v-model:value="formConfig.spec_type" :options="specTypeOptions" placeholder="自动检测" clearable />
        </n-form-item>
        <n-text v-if="!settingsStore.modelCapabilities.has_mtp" depth="3" style="font-size: 12px; margin-top: -12px; display: block; margin-bottom: 8px;">
          当前模型不支持 MTP，draft-mtp 选项已隐藏
        </n-text>
        <template v-if="formConfig.spec_type === 'draft-eagle3' || formConfig.spec_type === 'draft-simple'">
          <n-text v-if="!formConfig.spec_draft_model" type="warning" style="font-size: 12px; display: block; margin-bottom: 8px;">
            未配置 Draft 模型路径，推测解码将无法工作
          </n-text>
          <n-form-item>
            <template #label>Draft 模型路径 <HelpTip content="Eagle3 或 Draft 草稿模型的 .gguf 文件路径。仅 draft-eagle3/draft-simple 模式需要" /></template>
            <n-input v-model:value="formConfig.spec_draft_model" placeholder="Eagle3/Draft 草稿模型文件路径" @blur="autoSave" />
          </n-form-item>
          <n-form-item>
            <template #label>Draft GPU 层数 <HelpTip content="草稿模型加载到 GPU 的层数。0=全部用 CPU，100=全部用 GPU。建议与主模型一致以保证加速效果" /></template>
            <n-input-number v-model:value="formConfig.spec_draft_ngl" :min="0" :max="100" :step="1" placeholder="0" style="width: 100%" />
          </n-form-item>
          <n-form-item>
            <template #label>Draft 设备 <HelpTip content="草稿模型使用的 GPU 设备。留空自动选择，多卡场景可指定如 cuda:0" /></template>
            <n-input v-model:value="formConfig.spec_draft_device" placeholder="留空自动选择，如 cuda:0" @blur="autoSave" />
          </n-form-item>
        </template>
        <n-form-item v-if="formConfig.spec_type === 'draft-mtp' || formConfig.spec_type === 'draft-eagle3' || formConfig.spec_type === 'draft-simple'">
          <template #label>最大 Draft Token 数 <HelpTip content="每次推测最多生成的 token 数。值越大潜在加速越快但准确率下降，建议 3-4" /></template>
          <n-input-number v-model:value="formConfig.spec_draft_n_max" :min="1" :max="4" :step="1" placeholder="3" @blur="autoSave" />
        </n-form-item>
        <n-form-item v-if="formConfig.spec_type === 'draft-mtp' || formConfig.spec_type === 'draft-eagle3' || formConfig.spec_type === 'draft-simple'">
          <template #label>最小 Draft Token 数 <HelpTip content="每次推测最少生成的 token 数。0=不限制，设置后即使准确率低也会生成此数量的 token" /></template>
          <n-input-number v-model:value="formConfig.spec_draft_n_min" :min="0" :max="4" :step="1" placeholder="0" @blur="autoSave" />
        </n-form-item>
        <template v-if="formConfig.spec_type === 'ngram-mod'">
          <n-form-item>
            <template #label>N-Min <HelpTip content="ngram-mod 推测的最小 n-gram 长度。值越小匹配越宽松，建议 48" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_mod_n_min" :min="1" :max="256" :step="1" placeholder="48" @blur="autoSave" />
          </n-form-item>
          <n-form-item>
            <template #label>N-Max <HelpTip content="ngram-mod 推测的最大 n-gram 长度。值越大覆盖范围越广，建议 64" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_mod_n_max" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
          </n-form-item>
          <n-form-item>
            <template #label>N-Match <HelpTip content="ngram-mod 触发推测所需的最小匹配数。值越大越保守，建议 24" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_mod_n_match" :min="1" :max="256" :step="1" placeholder="24" @blur="autoSave" />
          </n-form-item>
        </template>
        <template v-if="formConfig.spec_type === 'ngram-simple'">
          <n-form-item>
            <template #label>Size-N <HelpTip content="ngram-simple 的 n-gram 长度。控制匹配窗口大小，建议 64" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_simple_size_n" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
          </n-form-item>
          <n-form-item>
            <template #label>Size-M <HelpTip content="ngram-simple 的 m-gram 长度。控制预测窗口大小，建议 64" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_simple_size_m" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
          </n-form-item>
          <n-form-item>
            <template #label>Min-Hits <HelpTip content="ngram-simple 触发推测所需的最小命中次数。值越大越保守，建议 1" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_simple_min_hits" :min="1" :max="256" :step="1" placeholder="1" @blur="autoSave" />
          </n-form-item>
        </template>
        <template v-if="formConfig.spec_type === 'ngram-map-k'">
          <n-form-item>
            <template #label>Size-N <HelpTip content="ngram-map-k 的 n-gram 长度。控制匹配窗口大小，建议 64" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_map_k_size_n" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
          </n-form-item>
          <n-form-item>
            <template #label>Size-M <HelpTip content="ngram-map-k 的 m-gram 长度。控制预测窗口大小，建议 64" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_map_k_size_m" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
          </n-form-item>
          <n-form-item>
            <template #label>Min-Hits <HelpTip content="ngram-map-k 触发推测所需的最小命中次数。值越大越保守，建议 1" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_map_k_min_hits" :min="1" :max="256" :step="1" placeholder="1" @blur="autoSave" />
          </n-form-item>
        </template>
        <template v-if="formConfig.spec_type === 'ngram-map-k4v'">
          <n-form-item>
            <template #label>Size-N <HelpTip content="ngram-map-k4v 的 n-gram 长度。控制匹配窗口大小，建议 64" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_map_k4v_size_n" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
          </n-form-item>
          <n-form-item>
            <template #label>Size-M <HelpTip content="ngram-map-k4v 的 m-gram 长度。控制预测窗口大小，建议 64" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_map_k4v_size_m" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
          </n-form-item>
          <n-form-item>
            <template #label>Min-Hits <HelpTip content="ngram-map-k4v 触发推测所需的最小命中次数。值越大越保守，建议 1" /></template>
            <n-input-number v-model:value="formConfig.spec_ngram_map_k4v_min_hits" :min="1" :max="256" :step="1" placeholder="1" @blur="autoSave" />
          </n-form-item>
        </template>
        <template v-if="formConfig.spec_type === 'ngram-cache'">
          <n-form-item>
            <template #label>静态缓存路径 <HelpTip content="预构建的静态 lookup 缓存文件路径。可显著加速推测解码，通常由训练工具生成" /></template>
            <n-input v-model:value="formConfig.lookup_cache_static" placeholder="lookup-cache-static 文件路径" @blur="autoSave" />
          </n-form-item>
          <n-form-item>
            <template #label>动态缓存路径 <HelpTip content="运行时生成的动态 lookup 缓存文件路径。会在推理过程中自动更新，加速后续请求" /></template>
            <n-input v-model:value="formConfig.lookup_cache_dynamic" placeholder="lookup-cache-dynamic 文件路径" @blur="autoSave" />
          </n-form-item>
        </template>

        <n-divider>RAG 重排序配置</n-divider>
        <n-form-item>
          <template #label>Reranker 模型 <HelpTip content="RAG 检索结果重排序使用的模型路径（.gguf 文件）。留空则不启用重排序，使用向量检索的原始排序" /></template>
          <n-input v-model:value="formConfig.reranker_model_path" placeholder="Reranker 模型文件路径（如 models/bge-reranker-v2-m3.gguf）" @blur="autoSave" />
        </n-form-item>
        <n-form-item>
          <template #label>重排序 Top N <HelpTip content="重排序后保留的文档数量。值越大召回越全但耗时越长，建议 3-5" /></template>
          <n-input-number v-model:value="formConfig.rerank_top_n" :min="1" :max="20" :step="1" placeholder="5" style="width: 100%" />
        </n-form-item>

        <n-divider>KV 缓存持久化</n-divider>
        <n-form-item>
          <template #label>启用 KV 缓存持久化 <HelpTip content="开启后将对话的 KV 缓存保存到磁盘，下次加载相同上下文时可跳过预填充，加快首 token 响应速度" /></template>
          <n-switch v-model:value="formConfig.slot_save_enabled" />
        </n-form-item>
        <n-form-item v-if="formConfig.slot_save_enabled">
          <template #label>缓存保存路径 <HelpTip content="KV 缓存保存到磁盘的目录路径。留空则使用默认路径（appDir/slots/）" /></template>
          <n-input v-model:value="formConfig.slot_save_path" placeholder="留空则使用默认路径（appDir/slots/）" @blur="autoSave" />
        </n-form-item>

        <n-divider>Agent 与 MCP</n-divider>
        <n-form-item>
          <template #label>Agent 模式 <HelpTip content="一键启用 CORS 代理和所有内置工具（文件读写、shell 命令等）。实验性功能，不建议在不可信环境启用" /></template>
          <n-switch v-model:value="formConfig.agent" :disabled="formConfig.ui_mcp_proxy" @update:value="handleAgentChange" />
        </n-form-item>
        <n-form-item>
          <template #label>MCP CORS 代理 <HelpTip content="仅为 Web UI 的 MCP 功能启用 CORS 代理。Agent 模式已包含此项" /></template>
          <n-switch v-model:value="formConfig.ui_mcp_proxy" :disabled="formConfig.agent" @update:value="autoSave" />
        </n-form-item>

        <n-divider>LoRA 适配器管理</n-divider>
        <LoraManager v-model:loraPaths="formConfig.lora_paths" @update:loraPaths="autoSave" />
      </n-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, onUnmounted, defineComponent, h } from 'vue'
import {
  NButton, NIcon, NForm, NFormItem, NInput,
  NSlider, NDivider, useMessage, NUpload,
  NSwitch, NInputNumber, NSelect, NTooltip,
} from 'naive-ui'
import { ArrowBackOutline } from '@vicons/ionicons5'
import { useSettingsStore } from '../stores/settings'
import { matchModelRef } from '../stores/settings'
import { MODEL_REFS } from '../utils/modelRefs'
import { type Config, type SearchAPIKeys } from '../services/wails'
import { wails } from '../services/wails'
import defaultUserAvatar from '../assets/images/user-avatar.svg'
import defaultAiAvatar from '../assets/images/appicon.png'
import LoraManager from '../components/LoraManager.vue'

const HelpTip = defineComponent({
  props: { content: String },
  setup(props) {
    return () => h(NTooltip, { trigger: 'hover' }, {
      trigger: () => h('span', { class: 'help-tip-icon' }, '?'),
      default: () => props.content
    })
  }
})

const settingsStore = useSettingsStore()
const message = useMessage()
const saving = ref(false)
const genParamsDirty = ref(false)
let genParamsSaveTimer: ReturnType<typeof setTimeout> | null = null

// GPU 检测结果：默认 true（显示全部选项），onMounted 时通过 getSmartParams 更新
const hasGPUInfo = ref(true)

const contextSizeSteps = [2048, 4096, 8192, 16384, 32768, 65536, 131072, 262144]
const contextSizeMarks: Record<number, string> = {
  0: '2K',
  1: '4K',
  2: '8K',
  3: '16K',
  4: '32K',
  5: '64K',
  6: '128K',
  7: '256K',
}

function formatContextSize(size: number): string {
  if (size >= 1024) {
    const k = size / 1024
    return k >= 1024 ? `${(k / 1024).toFixed(0)}M` : `${k >= 100 ? Math.round(k) : k}K`
  }
  return `${size}`
}

function findClosestStepIndex(size: number): number {
  let closest = 0
  let minDiff = Math.abs(contextSizeSteps[0] - size)
  for (let i = 1; i < contextSizeSteps.length; i++) {
    const diff = Math.abs(contextSizeSteps[i] - size)
    if (diff < minDiff) {
      minDiff = diff
      closest = i
    }
  }
  return closest
}

const contextSizeIndex = ref(2)
const refShowThinking = ref(false)

watch(contextSizeIndex, (idx) => {
  formConfig.value.context_size = contextSizeSteps[idx]
})

function applyContextSizeRef() {
  const raw = activeModelRefRaw.value
  const idx = findClosestStepIndex(raw.context_size)
  contextSizeIndex.value = idx
  formConfig.value.context_size = contextSizeSteps[idx]
}

const cacheTypeKOptions = computed(() => {
  const hasGPU = hasGPUInfo.value
  const baseOptions = [
    { label: '自动', value: '' },
    { label: 'f32 (32bit)', value: 'f32' },
    { label: 'f16 (16bit)', value: 'f16' },
    { label: 'q8_0 (8bit)', value: 'q8_0' },
    { label: 'q5_1 (5bit)', value: 'q5_1' },
    { label: 'q5_0 (5bit)', value: 'q5_0' },
    { label: 'q4_1 (4bit)', value: 'q4_1' },
    { label: 'q4_0 (4bit)', value: 'q4_0' },
  ]
  if (hasGPU) {
    // GPU 模式：在 f16 后插入 bf16，在 q4_0 后追加 iq4_nl
    baseOptions.splice(3, 0, { label: 'bf16 (16bit)', value: 'bf16' })
    baseOptions.push({ label: 'iq4_nl (4bit)', value: 'iq4_nl' })
  }
  return baseOptions
})

const cacheTypeVOptions = computed(() => {
  const hasGPU = hasGPUInfo.value
  const baseOptions = [
    { label: '自动', value: '' },
    { label: 'f32 (32bit)', value: 'f32' },
    { label: 'f16 (16bit)', value: 'f16' },
    { label: 'q8_0 (8bit)', value: 'q8_0' },
    { label: 'q5_1 (5bit)', value: 'q5_1' },
    { label: 'q5_0 (5bit)', value: 'q5_0' },
    { label: 'q4_1 (4bit)', value: 'q4_1' },
    { label: 'q4_0 (4bit)', value: 'q4_0' },
  ]
  if (hasGPU) {
    baseOptions.splice(3, 0, { label: 'bf16 (16bit)', value: 'bf16' })
    baseOptions.push({ label: 'iq4_nl (4bit)', value: 'iq4_nl' })
  }
  return baseOptions
})

const reasoningOptions = [
  { label: '开启', value: 'on' },
  { label: '关闭', value: 'off' },
  { label: '自动', value: 'auto' },
]

const specTypeOptions = computed(() => {
  const caps = settingsStore.modelCapabilities
  const options = [
    { label: '自动检测', value: '' },
  ]
  // 仅当模型支持 MTP 时才显示 draft-mtp 选项
  if (caps.has_mtp) {
    options.push({ label: 'MTP 推测解码 🔥', value: 'draft-mtp' })
  }
  options.push(
    { label: 'Eagle3 推测解码', value: 'draft-eagle3' },
    { label: 'Draft-Simple 推测解码', value: 'draft-simple' },
    { label: 'Ngram-Mod 推测解码', value: 'ngram-mod' },
    { label: 'Ngram-Simple 推测解码', value: 'ngram-simple' },
    { label: 'Ngram-Map-K 推测解码', value: 'ngram-map-k' },
    { label: 'Ngram-Map-K4V 推测解码', value: 'ngram-map-k4v' },
    { label: 'Ngram-Cache 推测解码', value: 'ngram-cache' },
    { label: '关闭', value: 'none' },
  )
  return options
})

const supportsReasoning = computed(() => settingsStore.modelCapabilities.reasoning)

const formConfig = ref<Config>({
  model_path: '',
  llama_server_path: '',
  api_base: '',
  port: 8080,
  context_size: 8192,
  temperature: 0.6,
  top_p: 0.95,
  top_k: 20,
  repeat_penalty: 1,
  mmproj_auto: true,
  mmproj_offload: true,
  kv_unified: false,
  cache_idle_slots: false,
  cache_ram: 0,
  image_min_tokens: 0,
  image_max_tokens: 0,
  fit_target: 0,
  fit_ctx: 0,
  system_prompt: '',
  chat_background: '',
  chat_background_opacity: 0.8,
  user_avatar: '',
  ai_avatar: '',
  search_mode: 'off',
  thinking_enabled: true,
  thinking_soft_switch: 'auto',
  sleep_idle_seconds: 120,
  models_max: 1,
  rag_enabled: false,
  rag_active_kb: 'default',
  rag_top_k: 3,
  rag_min_score: 0.3,
  rag_chunk_size: 512,
  rag_chunk_overlap: 64,
  embedding_model: '',
  mmap: true,
  kv_offload: true,
  context_shift: false,
  min_p: 0.05,
  dry_multiplier: 0,
  dry_base: 1.75,
  dry_allowed_length: 2,
  device: '',
  parallel: 0,
  cache_type_k: '',
  cache_type_v: '',
  spec_type: '',
  spec_draft_n_max: 0,
  spec_draft_n_min: 0,
  spec_ngram_mod_n_min: 0,
  spec_ngram_mod_n_max: 0,
  spec_ngram_mod_n_match: 0,
  spec_ngram_simple_size_n: 0,
  spec_ngram_simple_size_m: 0,
  spec_ngram_simple_min_hits: 0,
  spec_ngram_map_k_size_n: 0,
  spec_ngram_map_k_size_m: 0,
  spec_ngram_map_k_min_hits: 0,
  spec_ngram_map_k4v_size_n: 0,
  spec_ngram_map_k4v_size_m: 0,
  spec_ngram_map_k4v_min_hits: 0,
  lookup_cache_static: '',
  lookup_cache_dynamic: '',
  spec_draft_model: '',
  cache_type_k_draft: '',
  cache_type_v_draft: '',
  server_api_key_enabled: true,
  expose_server: false,
  swa_full: false,
  ctx_checkpoints: 0,
  checkpoint_min_step: 0,
  tools: '',
  prefill_assistant: false,
  slot_prompt_similarity: 0.8,
  skip_chat_parsing: false,
  api_prefix: '',
  simple_io: false,
  agent: false,
  ui_mcp_proxy: false,
  lora_paths: '',
  gpu_layers: 0,
  flash_attn: null,
  mlock: null,
  threads: 0,
  batch_size: 0,
  close_action: 'ask',
  // 推理配置
  reasoning: 'off',
  reasoning_budget: 0,
  reasoning_budget_message: '',
  reasoning_format: '',
  // RAG 重排序配置
  reranker_model_path: '',
  rerank_top_n: 5,
  // KV 缓存持久化配置
  slot_save_path: '',
  slot_save_enabled: false,
  // Draft 模型 GPU 配置
  spec_draft_ngl: 0,
  spec_draft_device: '',
})

const backgroundImageUrl = computed(() => {
    const bg = formConfig.value.chat_background
    if (!bg) return ''
    if (bg.startsWith('data:')) return bg
    return '/local-file/' + encodeURIComponent(bg)
})

// 搜索 API Key 设置状态（后端不再返回实际密钥，仅返回是否已设置）
const searchKeys = ref<SearchAPIKeys>({
    ollama_api_key: '',
    tavily_api_key: '',
    ollama_api_key_set: false,
    tavily_api_key_set: false,
})

// 用户输入的新 API Key（不在状态中保存真实密钥）
const newOllamaApiKey = ref('')
const newTavilyApiKey = ref('')

function saveSearchKeys() {
    // 只发送非空的 key，空值表示不更新
    const keysToUpdate: Partial<SearchAPIKeys> = {}
    if (newOllamaApiKey.value) {
        keysToUpdate.ollama_api_key = newOllamaApiKey.value
    }
    if (newTavilyApiKey.value) {
        keysToUpdate.tavily_api_key = newTavilyApiKey.value
    }
    if (Object.keys(keysToUpdate).length === 0) return
    settingsStore.saveSearchAPIKeys(keysToUpdate).then(() => {
        // 保存成功后清空输入框
        newOllamaApiKey.value = ''
        newTavilyApiKey.value = ''
    })
}

const serverApiKey = ref('')
const hasServerApiKey = ref(false)

function saveServerApiKey() {
    if (serverApiKey.value) {
        settingsStore.saveServerAPIKey(serverApiKey.value)
        hasServerApiKey.value = true
    }
}

async function onServerAPIKeyToggle() {
    await autoSave()
    // 切换开关后需要重新创建 client 以更新 API Key 设置
    if (formConfig.value.server_api_key_enabled) {
        hasServerApiKey.value = await settingsStore.hasServerAPIKey()
    }
}

async function onExposeServerToggle() {
    await autoSave()
    message.destroyAll()
    if (formConfig.value.expose_server) {
        message.warning('已开启局域网访问，重启服务后生效。请确保已设置 API Key 防止未授权访问。', { duration: 5000 })
    } else {
        message.info('已关闭局域网访问，重启服务后仅本机可访问。', { duration: 3000 })
    }
}

const currentModelRef = computed(() => {
  return matchModelRef(settingsStore.currentModel, MODEL_REFS)
})

const activeModelRefRaw = computed(() => {
  const ref = currentModelRef.value
  if (!ref) return { temperature: 0.6, top_p: 0.95, top_k: 20, context_size: 8192, repeat_penalty: 1.0 }
  const useThinking = settingsStore.thinkingEnabled && ref.raw_thinking
  return useThinking ? ref.raw_thinking! : ref.raw
})

function applyModelRef() {
  const ref = currentModelRef.value
  if (!ref) return
  const useThinking = settingsStore.thinkingEnabled && ref.raw_thinking
  const raw = useThinking ? ref.raw_thinking! : ref.raw
  formConfig.value.temperature = raw.temperature
  formConfig.value.top_p = raw.top_p
  formConfig.value.top_k = raw.top_k
  formConfig.value.repeat_penalty = raw.repeat_penalty
  const idx = findClosestStepIndex(raw.context_size)
  contextSizeIndex.value = idx
  formConfig.value.context_size = contextSizeSteps[idx]
  const modeLabel = useThinking ? '思考模式' : '非思考模式'
  message.destroyAll()
  message.success(`已应用 ${ref.name} ${modeLabel}参考参数`)
}

function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = reject
    reader.readAsDataURL(file)
  })
}

async function selectBackgroundImage() {
  try {
    const filePath = await wails.selectImageFile()
    if (filePath) {
      formConfig.value.chat_background = filePath
    }
  } catch {
    message.destroyAll()
    message.error('选择图片失败')
  }
}

function clearBackground() {
  formConfig.value.chat_background = ''
  formConfig.value.chat_background_opacity = 0.8
}

async function handleUserAvatarUpload(data: any) {
  const file = data.file.file as File
  if (file.size > 1024 * 1024) {
    message.destroyAll()
    message.error('头像图片大小不能超过 1MB')
    return
  }
  try {
    const base64 = await fileToBase64(file)
    formConfig.value.user_avatar = base64
  } catch {
    message.destroyAll()
    message.error('上传失败')
  }
}

function clearUserAvatar() {
  formConfig.value.user_avatar = ''
}

async function handleAIAvatarUpload(data: any) {
  const file = data.file.file as File
  if (file.size > 1024 * 1024) {
    message.destroyAll()
    message.error('头像图片大小不能超过 1MB')
    return
  }
  try {
    const base64 = await fileToBase64(file)
    formConfig.value.ai_avatar = base64
  } catch {
    message.destroyAll()
    message.error('上传失败')
  }
}

function clearAIAvatar() {
  formConfig.value.ai_avatar = ''
}

onMounted(async () => {
  await settingsStore.loadConfig()
  formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
  contextSizeIndex.value = findClosestStepIndex(formConfig.value.context_size)
  genParamsDirty.value = false
  await settingsStore.loadSearchAPIKeys()
  searchKeys.value = { ...settingsStore.searchAPIKeys }
  hasServerApiKey.value = await settingsStore.hasServerAPIKey()
  // 获取硬件信息以判断是否有 GPU（影响 KV cache 类型可选项）
  try {
    const smartParams = await wails.getSmartParams()
    hasGPUInfo.value = smartParams.hardware.has_gpu
  } catch {
    // 获取失败时保持默认值（true），显示全部选项
  }
})

watch(() => settingsStore.currentModel, async () => {
  if (!genParamsDirty.value) {
    await settingsStore.loadConfig()
    formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
    contextSizeIndex.value = findClosestStepIndex(formConfig.value.context_size)
    if (currentModelRef.value) {
      applyModelRef()
    }
    // 如果当前 spec_type 为 draft-mtp 但模型不支持 MTP，自动重置为空（自动检测）
    if (formConfig.value.spec_type === 'draft-mtp' && !settingsStore.modelCapabilities.has_mtp) {
      formConfig.value.spec_type = ''
    }
    // 非推理模型：自动重置 reasoning 为 off
    if (!settingsStore.modelCapabilities.reasoning && formConfig.value.reasoning !== 'off') {
      formConfig.value.reasoning = 'off'
      formConfig.value.reasoning_budget = -1
    }
  }
})

// GPU 状态变化时，自动重置不兼容的 KV cache 类型选中值（bf16/iq4_nl 仅 GPU 可用）
watch(hasGPUInfo, (hasGPU) => {
  if (!hasGPU) {
    const kVal = formConfig.value.cache_type_k
    const vVal = formConfig.value.cache_type_v
    if (kVal === 'bf16' || kVal === 'iq4_nl') {
      formConfig.value.cache_type_k = ''
    }
    if (vVal === 'bf16' || vVal === 'iq4_nl') {
      formConfig.value.cache_type_v = ''
    }
  }
})

const ALL_CONFIG_KEYS: (keyof Config)[] = [
  'model_path', 'llama_server_path', 'api_base', 'port', 'context_size',
  'temperature', 'top_p', 'top_k', 'repeat_penalty',
  'mmproj_auto', 'mmproj_offload', 'kv_unified', 'cache_idle_slots', 'cache_ram',
  'image_min_tokens', 'image_max_tokens', 'fit_target', 'fit_ctx',
  'system_prompt', 'chat_background', 'chat_background_opacity', 'user_avatar', 'ai_avatar',
  'search_mode', 'thinking_enabled', 'thinking_soft_switch', 'sleep_idle_seconds', 'models_max',
  'rag_enabled', 'rag_active_kb', 'rag_top_k', 'rag_min_score', 'rag_chunk_size', 'rag_chunk_overlap', 'embedding_model',
  'mmap', 'kv_offload', 'context_shift', 'min_p',
  'dry_multiplier', 'dry_base', 'dry_allowed_length',
  'device', 'parallel', 'cache_type_k', 'cache_type_v', 'spec_type',
  'spec_draft_n_max', 'spec_draft_n_min',
  'spec_ngram_mod_n_min', 'spec_ngram_mod_n_max', 'spec_ngram_mod_n_match',
  'spec_ngram_simple_size_n', 'spec_ngram_simple_size_m', 'spec_ngram_simple_min_hits',
  'spec_ngram_map_k_size_n', 'spec_ngram_map_k_size_m', 'spec_ngram_map_k_min_hits',
  'spec_ngram_map_k4v_size_n', 'spec_ngram_map_k4v_size_m', 'spec_ngram_map_k4v_min_hits',
  'lookup_cache_static', 'lookup_cache_dynamic', 'spec_draft_model',
  'cache_type_k_draft', 'cache_type_v_draft',
  'server_api_key_enabled', 'expose_server', 'swa_full',
  'ctx_checkpoints', 'checkpoint_min_step', 'tools', 'prefill_assistant',
  'slot_prompt_similarity', 'skip_chat_parsing', 'api_prefix', 'simple_io',
  'agent', 'ui_mcp_proxy',
  'gpu_layers', 'flash_attn', 'mlock', 'threads', 'batch_size',
  // 推理配置
  'reasoning', 'reasoning_budget', 'reasoning_budget_message', 'reasoning_format',
  // RAG 重排序配置
  'reranker_model_path', 'rerank_top_n',
  // KV 缓存持久化配置
  'slot_save_path', 'slot_save_enabled',
  // Draft 模型 GPU 配置
  'spec_draft_ngl', 'spec_draft_device',
]

watch(
  () => ALL_CONFIG_KEYS.map(k => formConfig.value[k]),
  () => {
    const savedConfig = settingsStore.config
    const dirty = ALL_CONFIG_KEYS.some(k => formConfig.value[k] !== savedConfig[k])
    genParamsDirty.value = dirty
    if (dirty) {
      scheduleAutoSave()
    }
  }
)

function scheduleAutoSave() {
  if (genParamsSaveTimer) {
    clearTimeout(genParamsSaveTimer)
  }
  genParamsSaveTimer = setTimeout(() => {
    autoSave()
  }, 1500)
}

// Agent 模式切换处理：启用 Agent 时自动关闭 UIMcpProxy（互斥）
function handleAgentChange() {
  if (formConfig.value.agent) {
    formConfig.value.ui_mcp_proxy = false
  }
  autoSave()
}

async function autoSave() {
  if (genParamsSaveTimer) {
    clearTimeout(genParamsSaveTimer)
    genParamsSaveTimer = null
  }
  saving.value = true
  try {
    await settingsStore.updateConfig(formConfig.value)
    formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
    genParamsDirty.value = false
  } catch {
    message.destroyAll()
    message.error('保存失败')
  } finally {
    saving.value = false
  }
}

onUnmounted(() => {
  if (genParamsSaveTimer) {
    clearTimeout(genParamsSaveTimer)
    genParamsSaveTimer = null
  }
})

</script>

<style scoped>
.settings-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary);
}

.settings-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-color);
}

.settings-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.settings-content {
  flex: 1;
  overflow-y: auto;
  padding: 20px 20px 80px;
  max-width: 640px;
  width: 100%;
  margin: 0 auto;
  /* 自定义滚动条，避免占用内容空间 */
  scrollbar-width: thin;
  scrollbar-color: var(--border-color) transparent;
}

/* WebKit 滚动条优化 */
.settings-content::-webkit-scrollbar {
  width: 6px;
}

.settings-content::-webkit-scrollbar-track {
  background: transparent;
}

.settings-content::-webkit-scrollbar-thumb {
  background-color: var(--border-color);
  border-radius: 3px;
}

.settings-content::-webkit-scrollbar-thumb:hover {
  background-color: var(--text-tertiary, rgba(0, 0, 0, 0.3));
}

/* 分隔线间距优化，避免配置区域过于拥挤 */
.settings-content :deep(.n-divider) {
  margin-top: 24px;
  margin-bottom: 16px;
}

.settings-content :deep(.n-divider:first-child) {
  margin-top: 0;
}

/* 表单项间距优化 */
.settings-content :deep(.n-form-item) {
  margin-bottom: 16px;
}

.upload-wrapper {
  width: 100%;
}

.upload-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 32px;
  border: 2px dashed var(--border-color);
  border-radius: var(--border-radius-md);
  cursor: pointer;
  transition: all 0.2s;
}

.upload-placeholder:hover {
  border-color: var(--accent-primary);
  background: var(--accent-tertiary);
}

.upload-icon {
  font-size: 32px;
}

.upload-text {
  color: var(--text-secondary);
}

.upload-preview {
  position: relative;
  border-radius: var(--border-radius-md);
  overflow: hidden;
  cursor: pointer;
}

.background-preview {
  width: 100%;
  height: 160px;
  object-fit: cover;
  border-radius: var(--border-radius-md);
}

.upload-actions {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  padding: 8px;
  background: linear-gradient(transparent, rgba(0, 0, 0, 0.5));
  display: flex;
  justify-content: center;
  z-index: 2;
}

.hover-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.35s cubic-bezier(0.4, 0, 0.2, 1);
  pointer-events: none;
  border-radius: var(--border-radius-md);
  z-index: 1;
}

.upload-preview:hover .hover-overlay {
  opacity: 1;
}

.hover-hint {
  color: #ffffff;
  font-size: 18px;
  font-weight: 500;
  letter-spacing: 0.05em;
  text-shadow: 0 1px 4px rgba(0, 0, 0, 0.35);
  user-select: none;
}

:deep(.upload-actions .n-button.n-button--text) {
  color: #ffffff;
}

:deep(.upload-actions .n-button.n-button--text:hover) {
  color: var(--accent-primary);
  background: rgba(255, 255, 255, 0.15);
}

.avatar-upload-wrapper {
  display: flex;
  align-items: center;
  gap: 16px;
}

.avatar-buttons {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* API Key 输入行：输入框 + 状态标签水平排列 */
.api-key-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.api-key-input {
  flex: 1;
}

.api-key-hint {
  font-size: 12px;
  color: var(--n-text-color-3);
}

.setting-hint {
  font-size: 12px;
  color: var(--n-text-color-3);
  margin-left: 12px;
}

.avatar-preview {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  box-shadow: var(--shadow-md);
  transition: all 0.2s;
  flex-shrink: 0;
}

.avatar-preview:hover {
  transform: scale(1.05);
  box-shadow: var(--shadow-lg);
}

.avatar-preview.ai-avatar {
}

.avatar-preview .avatar-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: 50%;
  aspect-ratio: 1;
}

.slider-value {
  min-width: 48px;
  text-align: right;
  font-size: 13px;
  color: var(--text-secondary);
  margin-left: 12px;
}

.rounded-textarea :deep(.n-input__textarea-wrapper) {
  border-radius: var(--border-radius-lg);
}

.rounded-textarea :deep(.n-input__textarea) {
  border-radius: var(--border-radius-lg);
}

.rounded-textarea :deep(.n-input__border) {
  border-radius: var(--border-radius-lg);
}

.rounded-textarea :deep(.n-input__state-border) {
  border-radius: var(--border-radius-lg);
}

.reset-btn {
  margin-left: 4px;
  font-size: 11px;
  color: var(--text-muted);
  min-width: 32px;
}

.model-ref-card {
  margin-bottom: 16px;
  border-radius: var(--border-radius-md);
  border: 1px solid var(--border-color);
  background: var(--bg-secondary);
  overflow: hidden;
}

.model-ref-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-tertiary, var(--bg-primary));
}

.model-ref-icon {
  font-size: 14px;
}

.model-ref-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.model-ref-current {
  margin-left: auto;
  font-size: 11px;
  color: var(--text-muted);
  font-weight: 400;
}

.model-ref-body {
  padding: 10px 14px;
}

.model-ref-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 0;
  font-size: 12px;
}

.model-ref-label {
  color: var(--text-secondary);
}

.model-ref-value {
  color: var(--text-primary);
  font-weight: 500;
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
}

.model-ref-note {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--border-color);
  font-size: 11px;
  color: var(--text-muted);
  line-height: 1.5;
}

.model-ref-apply {
  width: 100%;
  border-top: 1px solid var(--border-color);
  border-radius: 0 0 var(--border-radius-md) var(--border-radius-md);
}

.model-ref-tabs {
  display: flex;
  border-bottom: 1px solid var(--border-color);
  padding: 0 14px;
}

.model-ref-tab {
  padding: 6px 14px;
  font-size: 12px;
  color: var(--text-secondary);
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  cursor: pointer;
  transition: all 0.2s;
}

.model-ref-tab.active {
  color: var(--accent-primary);
  border-bottom-color: var(--accent-primary);
  font-weight: 600;
}

.model-ref-tab:hover:not(.active) {
  color: var(--text-primary);
}

.gen-params-save-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  padding: 12px 0 4px;
}

.gen-params-status {
  font-size: 12px;
  color: var(--accent-warning);
}

.gen-params-status.saved {
  color: var(--accent-success);
}

.help-tip-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  font-size: 10px;
  font-weight: 600;
  color: var(--text-muted);
  background: var(--bg-tertiary, rgba(0,0,0,0.06));
  margin-left: 4px;
  cursor: help;
  vertical-align: middle;
  line-height: 1;
}
</style>


