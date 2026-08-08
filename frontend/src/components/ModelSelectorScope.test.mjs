import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const here = dirname(fileURLToPath(import.meta.url))
const srcRoot = resolve(here, '..')
const source = readFileSync(join(srcRoot, 'components/ModelSelector.vue'), 'utf8')

test('模型选择器声明 usageContext 并调用 selectable API', () => {
  assert.match(source, /usageContext\?:\s*['"]personal['"] \| ['"]team['"]/)
  assert.match(source, /usageContext:\s*['"]personal['"]/)
  assert.match(source, /listSelectableModels\(props\.modelType,\s*props\.usageContext\)/)
  assert.doesNotMatch(source, /await listModels\(\)/)
})

test('模型选择器展示系统、默认、我的模型标签', () => {
  assert.match(source, /model\.is_system/)
  assert.match(source, /model\.scope === ['"]user['"]/)
  assert.match(source, /model\.is_default/)
})
