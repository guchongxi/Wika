import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const here = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(join(here, 'index.ts'), 'utf8')

test('模型 API 暴露系统、个人和可选模型接口', () => {
  assert.match(source, /export function listSystemModels\(/)
  assert.match(source, /export function createSystemModel\(/)
  assert.match(source, /export function updateSystemModel\(/)
  assert.match(source, /export function deleteSystemModel\(/)
  assert.match(source, /export function setSystemModelVisibility\(/)
  assert.match(source, /export function setSystemDefaultModel\(/)
  assert.match(source, /export function listMyModels\(/)
  assert.match(source, /export function createMyModel\(/)
  assert.match(source, /export function listSelectableModels\(/)
})

test('可选模型接口携带 usage_context 参数', () => {
  assert.match(source, /\/api\/v1\/models\/selectable\?type=\$\{type\}&usage_context=\$\{usageContext\}/)
})
