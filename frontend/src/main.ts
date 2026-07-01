import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
import './style.css'
// 任务 38：虚拟滚动（实验性，feature flag 默认关闭）
// - 引入 vue-virtual-scroller 的样式
// - app.use 注册全局插件（MessageList 里仍按需局部导入组件，便于 TS 类型解析）
import VirtualScroller from 'vue-virtual-scroller'
import 'vue-virtual-scroller/dist/vue-virtual-scroller.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(VirtualScroller)
app.mount('#app')
