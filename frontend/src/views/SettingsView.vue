<template>
  <div class="settings-container">
    <!-- 顶栏：返回 + 页题（书名式衬线排版），右侧缀一句书斋小铭 -->
    <div class="settings-header">
      <button class="back-btn" type="button" aria-label="返回" @click="$router.push('/')">
        <svg width="20" height="20" viewBox="0 0 512 512" fill="none" aria-hidden="true">
          <path
            d="M244 400L100 256l144-144M120 256h292"
            stroke="currentColor"
            stroke-width="48"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </button>
      <span class="settings-title">设置</span>
      <span class="settings-motto">静置一隅 · 调校你的豆芽</span>
    </div>

    <div class="settings-body">
      <!-- 左侧：书房目录（§ 章节锚点导航，点击平滑滚动到对应分节） -->
      <nav class="book-toc" aria-label="设置章节目录">
        <div class="toc-caption">目 录</div>

        <!-- 章节搜索（仿 VS Code 设置搜索：输入即过滤，仅保留匹配章节） -->
        <div class="toc-search">
          <svg
            class="toc-search-icon"
            width="14"
            height="14"
            viewBox="0 0 512 512"
            fill="none"
            aria-hidden="true"
          >
            <path
              d="M221 80a141 141 0 100 282 141 141 0 000-282zM323 323L432 432"
              stroke="currentColor"
              stroke-width="48"
              stroke-linecap="round"
            />
          </svg>
          <input
            v-model="searchQuery"
            type="text"
            class="toc-search-input"
            placeholder="搜索设置…"
            aria-label="搜索设置"
            @keydown.esc="searchQuery = ''"
          />
          <button
            v-if="searchQuery"
            type="button"
            class="toc-search-clear"
            aria-label="清空搜索"
            @click="searchQuery = ''"
          >
            <svg width="12" height="12" viewBox="0 0 512 512" fill="none" aria-hidden="true">
              <path
                d="M368 368L144 144M368 144L144 368"
                stroke="currentColor"
                stroke-width="64"
                stroke-linecap="round"
              />
            </svg>
          </button>
        </div>

        <template v-if="searchQuery && matchedSections.length === 0">
          <div class="toc-empty">未找到匹配的设置</div>
        </template>
        <button
          v-for="s in matchedSections"
          :key="s.id"
          type="button"
          class="toc-item"
          :class="{ 'is-active': activeSection === s.id }"
          @click="scrollToSection(s.id)"
        >
          <span class="toc-no">§{{ s.no }}</span>
          <span class="toc-label">{{ s.title }}</span>
          <!-- 印章方点：仅当前所在章节显示 -->
          <span class="toc-seal" aria-hidden="true"></span>
        </button>
      </nav>

      <!-- 右侧：正文区（分节滚动，各节常驻渲染保证内部组件状态不丢失） -->
      <div ref="contentRef" class="settings-content">
        <n-form label-placement="left" label-width="120" :model="formConfig">
          <!-- ==================== §1 外观 ==================== -->
          <section
            v-show="matchesSection('appearance')"
            class="book-section"
            data-section="appearance"
          >
            <header class="section-head">
              <span class="section-no">§ 1</span>
              <h2 class="section-title">外观</h2>
              <span class="section-desc">主题、背景、头像</span>
            </header>
            <div class="section-body">
              <AppearanceSettings />
            </div>
          </section>

          <!-- ==================== §2 AI 对话 ==================== -->
          <section class="book-section" data-section="chat">
            <header class="section-head">
              <span class="section-no">§ 2</span>
              <h2 class="section-title">AI 对话</h2>
              <span class="section-desc">提示词、推理、生成参数、朗读</span>
            </header>
            <div class="section-body">
              <AIChatSettings />
            </div>
          </section>

          <!-- ==================== §3 联网搜索 ==================== -->
          <section v-show="matchesSection('search')" class="book-section" data-section="search">
            <header class="section-head">
              <span class="section-no">§ 3</span>
              <h2 class="section-title">联网搜索</h2>
              <span class="section-desc">搜索引擎、搜索密钥</span>
            </header>
            <div class="section-body">
              <WebSearchSettings />
            </div>
          </section>

          <!-- ==================== §4 性能 ==================== -->
          <section class="book-section" data-section="performance">
            <header class="section-head">
              <span class="section-no">§ 4</span>
              <h2 class="section-title">性能</h2>
              <span class="section-desc">GPU、后端、KV 缓存、推测解码</span>
            </header>
            <div class="section-body">
              <PerformanceSettings />
            </div>
          </section>

          <!-- ==================== §5 API 服务 ==================== -->
          <section class="book-section" data-section="api">
            <header class="section-head">
              <span class="section-no">§ 5</span>
              <h2 class="section-title">API 服务</h2>
              <span class="section-desc">端点、密钥、局域网访问</span>
            </header>
            <div class="section-body">
              <APIServiceSettings />
            </div>
          </section>

          <!-- ==================== §6 高级 ==================== -->
          <section v-show="matchesSection('advanced')" class="book-section" data-section="advanced">
            <header class="section-head">
              <span class="section-no">§ 6</span>
              <h2 class="section-title">高级</h2>
              <span class="section-desc">MCP 工具、RAG、LoRA、实验功能</span>
            </header>
            <div class="section-body">
              <AdvancedExperimentalSettings />
            </div>
          </section>

          <!-- ==================== §7 模型下载 ==================== -->
          <section class="book-section" data-section="model-download">
            <header class="section-head">
              <span class="section-no">§ 7</span>
              <h2 class="section-title">模型下载</h2>
              <span class="section-desc">ModelScope / HF 镜像内置下载</span>
            </header>
            <div class="section-body">
              <ModelDownloader />
            </div>
          </section>

          <!-- ==================== §8 关于 ==================== -->
          <section v-show="matchesSection('about')" class="book-section" data-section="about">
            <header class="section-head">
              <span class="section-no">§ 8</span>
              <h2 class="section-title">关于</h2>
              <span class="section-desc">版本信息与帮助反馈</span>
            </header>
            <div class="section-body">
              <AboutSettings />
            </div>
          </section>
        </n-form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  computed,
  defineAsyncComponent,
  nextTick,
  onMounted,
  onUnmounted,
  provide,
  ref,
  watch
} from 'vue'
import { useRoute } from 'vue-router'
import { NForm, useMessage } from 'naive-ui'
import AppearanceSettings from '../components/settings/AppearanceSettings.vue'
import AIChatSettings from '../components/settings/AIChatSettings.vue'
import WebSearchSettings from '../components/settings/WebSearchSettings.vue'
import PerformanceSettings from '../components/settings/PerformanceSettings.vue'
import APIServiceSettings from '../components/settings/APIServiceSettings.vue'
import AdvancedExperimentalSettings from '../components/settings/AdvancedExperimentalSettings.vue'
import AboutSettings from '../components/settings/AboutSettings.vue'
// 性能项：模型下载器体积大且非首屏必需，异步加载减小设置页首包
const ModelDownloader = defineAsyncComponent(
  () => import('../components/settings/ModelDownloader.vue')
)
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from '../components/settings/settingsContext'
import { useSettingsCore } from '../components/settings/composables/useSettingsCore'
import { useAppearanceSettings } from '../components/settings/composables/useAppearanceSettings'
import { useAIChatSettings } from '../components/settings/composables/useAIChatSettings'
import { usePerformanceSettings } from '../components/settings/composables/usePerformanceSettings'
import { useAPIServiceSettings } from '../components/settings/composables/useAPIServiceSettings'
// 章节数据与搜索匹配逻辑为纯函数（见 utils/sectionSearch.ts），与视图解耦便于单测
import { SETTINGS_SECTIONS, sectionMatches, filterSections } from '../utils/sectionSearch'

const message = useMessage()
const route = useRoute()

// ===== 书房目录章节数据与搜索匹配 =====
// 数据与纯逻辑已下沉至 utils/sectionSearch.ts（见其注释），此处仅保留视图层引用
const SECTIONS = SETTINGS_SECTIONS

// ===== 章节搜索 =====
const searchQuery = ref('')

/** 判断章节是否匹配当前搜索词（匹配标题/描述/关键词，不区分大小写） */
function matchesSection(id: string): boolean {
  const s = SECTIONS.find(x => x.id === id)
  if (!s) return false
  return sectionMatches(s, searchQuery.value)
}

/** 过滤后的目录项（搜索时仅展示匹配章节） */
const matchedSections = computed(() => filterSections(SECTIONS, searchQuery.value))

// 搜索词变化：自动定位到第一个匹配章节，避免用户滚动位置停留在被隐藏的章节上
watch(searchQuery, async () => {
  if (searchQuery.value.trim()) {
    activeSection.value = matchedSections.value[0]?.id ?? ''
    await nextTick()
    if (matchedSections.value.length > 0) scrollToSection(matchedSections.value[0].id)
  } else {
    activeSection.value = SECTIONS[0].id
  }
})

// 设置域重建：SettingsView 退化为编排壳，组装顺序即依赖链：
// core ← performance ← aiChat；appearance / apiService 仅依赖 core
const core = useSettingsCore(message)
const appearance = useAppearanceSettings(core, message)
const performance = usePerformanceSettings(core)
const aiChat = useAIChatSettings(core, performance, message)
const apiService = useAPIServiceSettings(core, message)

// 模板 n-form 的 :model 绑定需要表单对象
const { formConfig } = core

// 通过 provide 向子组件注入共享状态
const settingsContext: SettingsContext = { core, appearance, aiChat, performance, apiService }
provide(SETTINGS_CONTEXT_KEY, settingsContext)

// ===== 目录导航与滚动态 =====
const contentRef = ref<HTMLElement | null>(null)
// 当前所在章节（驱动目录印章方点与高亮）
const activeSection = ref<string>('appearance')
// 滚动同步用的 rAF 句柄（避免滚动事件高频触发布局查询）
let scrollRafId: number | null = null

/** 按 data-section 属性查找分节元素 */
function getSectionEl(id: string): HTMLElement | null {
  return contentRef.value?.querySelector(`[data-section="${id}"]`) ?? null
}

/**
 * 目录点击：平滑滚动到对应章节顶部（留一点呼吸空隙）。
 * 用 getBoundingClientRect 差分计算滚动距离，不依赖 offsetTop 的
 * offsetParent 定位链（该基准会随嵌套容器/定位祖先变化而错位），
 * 保证无论布局怎样都能算到准确目标位置。
 */
function scrollToSection(id: string) {
  const el = getSectionEl(id)
  const container = contentRef.value
  if (!el || !container) return
  // 点击即同步高亮（搜索态下 syncActiveSection 已暂停，必须在此显式更新）
  activeSection.value = id
  const delta = el.getBoundingClientRect().top - container.getBoundingClientRect().top
  container.scrollTo({ top: Math.max(container.scrollTop + delta - 8, 0), behavior: 'smooth' })
}

/**
 * 滚动位置 → 当前章节同步。
 * 判定线取内容视口顶部下方 140px，线上方最近的章节即为当前节；
 * 滚动抵达底部时兜底高亮最后一节（末节较短时无法越过判定线的场景）。
 */
function syncActiveSection() {
  // 搜索激活时：被 v-show 隐藏章节的 offsetTop 恒为 0，会污染高亮判定，
  // 故搜索态下固定高亮第一个匹配章节（由 searchQuery 的 watch 维护）。
  if (searchQuery.value.trim()) return
  const container = contentRef.value
  if (!container) return
  const line = container.scrollTop + 140
  let current = SECTIONS[0].id
  for (const s of SECTIONS) {
    const el = getSectionEl(s.id)
    if (el && el.offsetTop <= line) {
      current = s.id
    }
  }
  if (container.scrollTop + container.clientHeight >= container.scrollHeight - 8) {
    current = SECTIONS[SECTIONS.length - 1].id
  }
  activeSection.value = current
}

function handleContentScroll() {
  // rAF 节流：一帧最多做一次 offsetTop 读取与高亮更新
  if (scrollRafId !== null) return
  scrollRafId = requestAnimationFrame(() => {
    scrollRafId = null
    syncActiveSection()
  })
}

// 显式按序初始化（原 onMounted 逻辑等价迁移）：配置 → 性能/GPU → 密钥状态 → 模型预设
onMounted(async () => {
  contentRef.value?.addEventListener('scroll', handleContentScroll, { passive: true })
  syncActiveSection()

  await core.init()
  await performance.init()
  await apiService.init()
  await aiChat.init()

  // 首轮定位：等所有 init 与异步区块（ModelDownloader）渲染稳定后再滚动。
  // 双 rAF 缓冲 + 二次校准，避免 smooth 动画被中途布局变化打断停半路
  // （否则会落在紧邻的"高级"等章节顶部）。
  locateFromRoute()
  watch(() => route.query.open, locateFromRoute)
})

/**
 * 根据路由 query.open 定位到指定章节（如欢迎页"前往模型下载"带 open=model-download）。
 * 既服务首轮挂载，也服务组件复用后再跳转（onMounted 不会二次触发的场景）。
 * 双 rAF：先等当前帧布局稳定，再发起滚动；滚动结束后再校准一次兜底偏差。
 */
function locateFromRoute() {
  const openName = route.query.open
  if (!openName || typeof openName !== 'string') return
  if (!SECTIONS.some(s => s.id === openName)) return
  nextTick(() => {
    requestAnimationFrame(() => {
      scrollToSection(openName)
      requestAnimationFrame(() => scrollToSection(openName))
    })
  })
}

onUnmounted(() => {
  contentRef.value?.removeEventListener('scroll', handleContentScroll)
  if (scrollRafId !== null) {
    cancelAnimationFrame(scrollRafId)
    scrollRafId = null
  }
})
</script>

<style scoped>
.settings-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  /* B5 veil 结构层：与顶栏/输入区同层。背景图模式下整页变通透，
   * 表单可读性由组件自身表面保证；无背景图时 alpha=1 自动退化实色 */
  background: var(--surface-veil);
}

/* ===== 顶栏 ===== */
.settings-header {
  display: flex;
  align-items: center;
  gap: 12px;
  /* v4：顶部净空加大到 68px（52px 避让拖拽带 + 原 16px 内距） */
  padding: 68px 24px 16px;
  border-bottom: 1px solid var(--border-color);
}

/* .back-btn 样式在全局样式表中定义，此处只保留类名 */

.settings-title {
  font-family: var(--font-display);
  font-size: 22px;
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: 3px;
}

/* 书斋小铭：靠右的一句闲章小字，克制不抢焦点 */
.settings-motto {
  margin-left: auto;
  font-family: var(--font-display);
  font-size: 12px;
  color: var(--text-muted);
  letter-spacing: 2px;
}

/* ===== 双栏骨架：左目录 + 右正文 ===== */
.settings-body {
  flex: 1;
  display: flex;
  min-height: 0; /* 关键：允许 flex 子项收缩出内部滚动条 */
}

.book-toc {
  width: 200px;
  flex-shrink: 0;
  padding: 26px 14px 24px 24px;
  border-right: 1px solid var(--border-light);
  overflow-y: auto;
  scrollbar-width: none; /* 目录不长，藏起滚动条保持素净 */
}
.book-toc::-webkit-scrollbar {
  display: none;
}

.toc-caption {
  font-family: var(--font-display);
  font-size: 12px;
  color: var(--text-muted);
  letter-spacing: 6px;
  padding-left: 10px;
  margin-bottom: 14px;
}

/* 章节搜索框：书签签条样式的内联输入（仿 VS Code 设置搜索） */
.toc-search {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0 6px 12px;
  padding: 0 8px;
  height: 28px;
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-sm);
  background: var(--bg-tertiary);
  transition: border-color var(--transition-fast);
}
.toc-search:focus-within {
  border-color: var(--seal-color);
}

.toc-search-icon {
  color: var(--text-tertiary);
  flex-shrink: 0;
}

.toc-search-input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  color: var(--text-primary);
  font-size: 12px;
}
.toc-search-input::placeholder {
  color: var(--text-tertiary);
}

.toc-search-clear {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border: none;
  background: transparent;
  color: var(--text-tertiary);
  cursor: pointer;
  border-radius: 3px;
  transition:
    color var(--transition-fast),
    background-color var(--transition-fast);
}
.toc-search-clear:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

/* 无匹配的空态提示 */
.toc-empty {
  padding: 10px 14px;
  font-size: 12px;
  color: var(--text-muted);
}

.toc-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 7px 10px;
  border: none;
  background: transparent;
  border-radius: var(--border-radius-sm);
  cursor: pointer;
  color: var(--text-secondary);
  font-size: 13px;
  text-align: left;
  transition:
    background-color var(--transition-fast),
    color var(--transition-fast);
}
.toc-item:hover {
  /* 书房悬浮态：背景色阶变化即可，不加投影 */
  background: var(--bg-hover);
  color: var(--text-primary);
}
.toc-item.is-active {
  color: var(--text-primary);
  font-weight: 600;
}

.toc-no {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-muted);
  width: 24px;
  flex-shrink: 0;
  font-variant-numeric: tabular-nums;
}
.toc-item.is-active .toc-no {
  color: var(--seal-color);
}

.toc-label {
  flex: 1;
  font-family: var(--font-display);
  letter-spacing: 1px;
}

/* 印章方点：默认隐没，激活时以小尺寸浮现 */
.toc-seal {
  width: 7px;
  height: 7px;
  flex-shrink: 0;
  background: var(--seal-color);
  border-radius: 1px;
  opacity: 0;
  transform: scale(0.5);
  transition:
    opacity var(--transition-fast),
    transform var(--transition-fast);
}
.toc-item.is-active .toc-seal {
  opacity: 1;
  transform: scale(1);
}

/* ===== 正文滚动区 ===== */
.settings-content {
  flex: 1;
  position: relative; /* 子节 offsetTop 以此为基准，供滚动同步计算 */
  overflow-y: auto;
  padding: 32px 32px 96px 36px;
  scrollbar-width: thin;
  scrollbar-color: var(--border-color) transparent;
}

/* Q10: 设置面板的滚动条有意低调于全局样式（细、灰、hover 才加深），不抢内容焦点 */
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
  background-color: var(--text-tertiary);
}

/* ===== 章节（分节正文） ===== */
.book-section {
  max-width: 680px; /* 行长上限，宽窗下右侧留白即书房气息 */
}

/* 相邻章节之间：自绘 1px hairline 分隔（替代 NDivider） */
.book-section + .book-section {
  margin-top: 48px;
  padding-top: 36px;
  border-top: 1px solid var(--border-light);
}

.section-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 20px;
}

.section-no {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--seal-color);
  font-variant-numeric: tabular-nums;
}

.section-title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 19px;
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: 2px;
  /* 章节标题为短词，非必要不折行 */
  white-space: nowrap;
}

.section-desc {
  font-size: 12px;
  color: var(--text-muted);
}

/* 表单项间距：统一节奏 */
.settings-content :deep(.n-form-item) {
  margin-bottom: 16px;
}

/* ===== 窄窗适配：目录退场，正文独占 ===== */
@media (max-width: 900px) {
  .book-toc {
    display: none;
  }
  .settings-content {
    padding: 24px 20px 80px;
  }
}
</style>
