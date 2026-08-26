import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
// 书房字体系统（可变字重，按需分包加载）：
// 思源宋体 = 标题/章节号（书卷气）/ Manrope = 拉丁正文与数字 /
// 思源黑体 = 中文正文 / JetBrains Mono = 代码与等宽数字
import '@fontsource-variable/manrope'
import '@fontsource-variable/noto-serif-sc'
import '@fontsource-variable/noto-sans-sc'
import '@fontsource-variable/jetbrains-mono'
import './styles/tokens.css'
import './style.css'
// 虚拟滚动（实验性，feature flag 默认关闭）
// - 引入 vue-virtual-scroller 的样式
// - app.use 注册全局插件（MessageList 里仍按需局部导入组件，便于 TS 类型解析）
import VirtualScroller from 'vue-virtual-scroller'
import 'vue-virtual-scroller/dist/vue-virtual-scroller.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(VirtualScroller)
app.mount('#app')
