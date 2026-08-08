import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const here = dirname(fileURLToPath(import.meta.url))
const srcRoot = resolve(here, '../../')

const readSrc = (relativePath) => readFileSync(join(srcRoot, relativePath), 'utf8')

test('用户菜单中的系统设置打开独立系统设置弹窗', () => {
  const source = readSrc('components/UserMenu.vue')
  const handler = source.match(/const handleSystemAdmin = \(\) => \{[\s\S]*?\n\}/)?.[0] || ''

  assert.match(handler, /uiStore\.openSystemSettings\(\)/)
  assert.doesNotMatch(handler, /openSettings\(['"]system-global['"]\)/)
  assert.doesNotMatch(handler, /router\.push\(\{\s*path:\s*['"]\/platform\/settings['"][\s\S]*system-global/)
})

test('UI store 暴露独立系统设置弹窗状态和动作', () => {
  const source = readSrc('stores/ui.ts')

  assert.match(source, /showSystemSettingsModal:\s*false/)
  assert.match(source, /systemSettingsInitialSection:\s*null as string \| null/)
  assert.match(source, /systemSettingsInitialSubSection:\s*null as string \| null/)
  assert.match(source, /openSystemSettings\(section\?: string, subSection\?: string\)/)
  assert.match(source, /closeSystemSettings\(\)/)
})

test('App 全局挂载独立系统设置弹窗', () => {
  const source = readSrc('App.vue')

  assert.match(source, /import SystemSettings from ['"]@\/views\/system\/SystemSettings\.vue['"]/)
  assert.match(source, /<SystemSettings\s*\/>/)
})

test('通用设置弹窗不再承载系统设置导航和内容', () => {
  const source = readSrc('views/settings/Settings.vue')

  assert.doesNotMatch(source, /import SystemSettings from ['"]@\/views\/system\/SystemSettings\.vue['"]/)
  assert.doesNotMatch(source, /<SystemSettings\s*\/>/)
  assert.doesNotMatch(source, /key:\s*['"]system-global['"]/)
  assert.doesNotMatch(source, /currentSection === ['"]system-global['"]/)
})

test('系统设置弹窗提供左侧纵向 tab 和可实施分类', () => {
  const source = readSrc('views/system/SystemSettings.vue')
  const expectedTabs = [
    'platform-runtime',
    'account-tenant',
    'kb-defaults',
    'models-services',
    'security-network',
    'governance',
    'system-admins',
  ]

  assert.match(source, /showSystemSettingsModal/)
  assert.match(source, /authStore\.isSystemAdmin/)
  assert.match(source, /system-settings-sidebar/)
  assert.match(source, /system-settings-nav/)

  for (const key of expectedTabs) {
    assert.match(source, new RegExp(`key:\\s*['"]${key}['"]`))
  }

  assert.match(source, /'platform-runtime'[\s\S]*'asynq\.concurrency'/)
  assert.match(source, /'account-tenant'[\s\S]*'auth\.registration_mode'[\s\S]*'tenant\.max_owned_per_user'[\s\S]*'tenant\.default_storage_quota_gb'/)
  assert.match(source, /'kb-defaults'[\s\S]*'kb\.default_llm_model_id'[\s\S]*'kb\.default_embedding_model_id'[\s\S]*'kb\.default_vlm_model_id'[\s\S]*'kb\.default_asr_model_id'/)
  assert.match(source, /'security-network'[\s\S]*'ssrf\.whitelist'/)
  assert.match(source, /'governance'[\s\S]*'wika\.governance\.conflict\.enabled'[\s\S]*'wika\.governance\.version\.enabled'[\s\S]*'wika\.governance\.url_refresh\.enabled'[\s\S]*'wika\.governance\.eval_schedule\.enabled'[\s\S]*'wika\.governance\.org_share\.enabled'/)
})

test('默认模型管理入口跳转到系统设置的模型与服务 tab', () => {
  const source = readSrc('views/system/SystemSettings.vue')

  assert.match(source, /@add-model="openModelSettings\(item\.key\)"/)
  assert.match(source, /relatedActionFor\(item\)/)
  assert.match(source, /openModelSettings\(key: string\)/)
  assert.match(source, /activeTab\.value\s*=\s*['"]models-services['"]/)
  assert.match(source, /<ModelSettings mode="system" :initial-type="activeModelSettingsType" \/>/)
  assert.doesNotMatch(source, /uiStore\.openSettings\(['"]models['"]/)
})
