import { describe, expect, it } from 'vitest'
import {
  SETTINGS_SECTIONS,
  sectionMatches,
  filterSections,
  type SettingsSection
} from './sectionSearch'

describe('sectionSearch 设置章节搜索', () => {
  it('章节数据包含 8 个设置分节且 id 唯一', () => {
    const ids = SETTINGS_SECTIONS.map(s => s.id)
    expect(ids).toHaveLength(8)
    expect(new Set(ids).size).toBe(8)
  })

  describe('sectionMatches 匹配语义', () => {
    it('空查询匹配所有章节', () => {
      for (const s of SETTINGS_SECTIONS) {
        expect(sectionMatches(s, '')).toBe(true)
        expect(sectionMatches(s, '   ')).toBe(true)
      }
    })

    it('按标题命中', () => {
      const performance = SETTINGS_SECTIONS.find(s => s.id === 'performance')!
      expect(sectionMatches(performance, '性能')).toBe(true)
    })

    it('按描述命中', () => {
      const api = SETTINGS_SECTIONS.find(s => s.id === 'api')!
      expect(sectionMatches(api, '局域网')).toBe(true)
    })

    it('按关键词命中', () => {
      const performance = SETTINGS_SECTIONS.find(s => s.id === 'performance')!
      expect(sectionMatches(performance, 'CUDA')).toBe(true)
      const advanced = SETTINGS_SECTIONS.find(s => s.id === 'advanced')!
      expect(sectionMatches(advanced, 'MCP')).toBe(true)
    })

    it('不区分大小写', () => {
      const download = SETTINGS_SECTIONS.find(s => s.id === 'model-download')!
      expect(sectionMatches(download, 'huggingface')).toBe(true)
      expect(sectionMatches(download, 'modelscope')).toBe(true)
    })

    it('无匹配返回 false', () => {
      for (const s of SETTINGS_SECTIONS) {
        expect(sectionMatches(s, '不存在的关键词xyz')).toBe(false)
      }
    })

    it('查询词首尾空白被忽略', () => {
      const performance = SETTINGS_SECTIONS.find(s => s.id === 'performance')!
      expect(sectionMatches(performance, '  CUDA  ')).toBe(true)
    })
  })

  describe('filterSections 过滤语义', () => {
    it('空查询返回全部且顺序不变', () => {
      expect(filterSections(SETTINGS_SECTIONS, '')).toEqual(SETTINGS_SECTIONS)
    })

    it('过滤后仅保留匹配章节', () => {
      const matched = filterSections(SETTINGS_SECTIONS, '温度')
      expect(matched.map(s => s.id)).toEqual(['chat'])
    })

    it('多个章节共享关键词时全部命中', () => {
      const matched = filterSections(SETTINGS_SECTIONS, '密钥')
      const ids = matched.map(s => s.id)
      expect(ids).toContain('search')
      expect(ids).toContain('api')
    })

    it('无匹配返回空数组', () => {
      expect(filterSections(SETTINGS_SECTIONS, 'zzz不存在')).toEqual([])
    })

    it('自定义列表同样生效（保持传入顺序）', () => {
      const subset: SettingsSection[] = [
        SETTINGS_SECTIONS[3],
        SETTINGS_SECTIONS[0],
        SETTINGS_SECTIONS[7]
      ]
      const matched = filterSections(subset, '外观')
      expect(matched.map(s => s.id)).toEqual(['appearance'])
    })
  })
})
