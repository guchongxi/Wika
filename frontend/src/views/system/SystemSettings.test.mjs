import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const here = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(join(here, 'SystemSettings.vue'), 'utf8')

test('系统默认模型设置使用模型选择器而不是自由文本输入', () => {
  assert.match(source, /import ModelSelector from ['"]@\/components\/ModelSelector\.vue['"]/)
  assert.match(source, /v-else-if="isModelSetting\(item\.key\)"/)
  assert.match(source, /:model-type="modelSettingType\(item\.key\)"/)
  assert.match(source, /@update:selected-model-id="\([^)]+\) => onModelSettingChange\(item, [^)]+\)"/)

  const selectorIndex = source.indexOf('v-else-if="isModelSetting(item.key)"')
  const freeTextInputMatch = source.match(/<t-input\s+v-else/)
  const freeTextInputIndex = freeTextInputMatch?.index ?? -1
  assert.ok(selectorIndex > -1, '缺少默认模型选择器分支')
  assert.ok(freeTextInputIndex > -1, '缺少自由文本兜底输入')
  assert.ok(selectorIndex < freeTextInputIndex, '默认模型设置必须在自由文本兜底前被拦截')
})
