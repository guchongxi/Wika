import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const here = dirname(fileURLToPath(import.meta.url))
const srcRoot = resolve(here, '../../')
const source = readFileSync(join(srcRoot, 'views/settings/ModelSettings.vue'), 'utf8')

test('模型设置支持 system 和 personal 两种模式', () => {
  assert.match(source, /mode\?:\s*['"]system['"] \| ['"]personal['"]/)
  assert.match(source, /mode:\s*['"]personal['"]/)
  assert.match(source, /props\.mode === ['"]system['"]/)
})

test('系统模式走系统模型 API 并允许管理员编辑系统模型', () => {
  assert.match(source, /listSystemModels\(/)
  assert.match(source, /createSystemModel\(/)
  assert.match(source, /updateSystemModel\(/)
  assert.match(source, /deleteSystemModel\(/)
  assert.match(source, /setSystemModelVisibility\(/)
  assert.match(source, /setSystemDefaultModel\(/)
  assert.match(source, /isSystemMode\.value && authStore\.isSystemAdmin/)
})

test('系统设置弹窗以系统模式嵌入模型设置', () => {
  const systemSource = readFileSync(join(srcRoot, 'views/system/SystemSettings.vue'), 'utf8')
  assert.match(systemSource, /<ModelSettings mode="system" :initial-type="activeModelSettingsType" \/>/)
})
