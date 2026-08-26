/**
 * vue-virtual-scroller（v2.0.0-beta，Vue 3 版）未附带 TypeScript 类型声明。
 * 此处提供宽松的类型 shim，仅用于让 vue-tsc 通过模板类型检查。
 * 类型不做精确约束，具体参数用法由调用处自行保证。
 *
 * 说明：DefineComponent 的 SlotsType 位于第 13 个泛型参数位，因此前面用 12 个 any
 * 占位，只为把默认插槽的 props 类型传进去，让模板里 `#default="{ item, index, active }"`
 * 解构能通过类型检查。
 */
declare module 'vue-virtual-scroller' {
  import type { DefineComponent, SlotsType } from 'vue'

  // DynamicScroller：动态高度虚拟滚动容器
  // 常用 props：items / keyField / minItemSize / buffer / pageMode
  // 默认插槽 props：{ item, index, active, itemWithSize }
  export const DynamicScroller: DefineComponent<
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    SlotsType<{ default: { item: any; index: number; active: boolean; itemWithSize: any } }>
  >

  // DynamicScrollerItem：包裹每个列表项，负责尺寸观测与回收协同
  // 常用 props：item / active / index / watchData / sizeDependencies / data-index(attr)
  // 默认插槽无 props
  export const DynamicScrollerItem: DefineComponent<
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    any,
    SlotsType<{ default: Record<string, never> }>
  >

  // RecycleScroller：定高虚拟滚动（本项目未使用，保留以防导入报错）
  export const RecycleScroller: DefineComponent<any, any, any>

  // 插件对象，供 app.use(VirtualScroller) 全局注册
  const VirtualScroller: { install: (app: import('vue').App) => void }
  export default VirtualScroller
}
