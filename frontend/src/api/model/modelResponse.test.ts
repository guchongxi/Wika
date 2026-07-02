import assert from 'node:assert/strict'
import test from 'node:test'

import { normalizeModelListResponse } from './modelResponse.ts'

const chatModel = {
  id: 'chat-1',
  name: 'chat',
  type: 'KnowledgeQA',
  source: 'remote',
  parameters: {},
}

const embeddingModel = {
  id: 'embedding-1',
  name: 'embedding',
  type: 'Embedding',
  source: 'remote',
  parameters: {},
}

test('模型列表兼容后端直接返回数组', () => {
  assert.deepEqual(
    normalizeModelListResponse([chatModel, embeddingModel]),
    [chatModel, embeddingModel],
  )
})

test('模型列表兼容旧版 success/data 包装格式', () => {
  assert.deepEqual(
    normalizeModelListResponse({
      success: true,
      data: [chatModel],
    }),
    [chatModel],
  )
})

test('模型列表按类型过滤', () => {
  assert.deepEqual(
    normalizeModelListResponse([chatModel, embeddingModel], 'Embedding'),
    [embeddingModel],
  )
})
