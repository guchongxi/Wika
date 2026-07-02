import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import test from 'node:test'

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(join(__dirname, 'UploadConfirmDialog.vue'), 'utf8')

test('上传确认弹窗层级高于全局抽屉浮层', () => {
  const match = source.match(/\.upload-confirm-overlay\s*\{[^}]*z-index:\s*(\d+)/s)

  assert.ok(match, '缺少 .upload-confirm-overlay 的 z-index 声明')
  assert.ok(
    Number(match[1]) >= 3600,
    '上传确认弹窗必须高于 TDesign 抽屉和全局下拉层级，避免在线编辑发布确认被遮挡',
  )
})
