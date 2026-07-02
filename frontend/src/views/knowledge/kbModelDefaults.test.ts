import assert from 'node:assert/strict'
import test from 'node:test'

import {
  getMissingExplicitKBModelKeys,
  pickPreferredKBModelId,
  type KBModelLike,
} from './kbModelDefaults.ts'

const chat = (id: string, extra: Partial<KBModelLike> = {}): KBModelLike => ({
  id,
  type: 'KnowledgeQA',
  ...extra,
})

const embedding = (id: string, extra: Partial<KBModelLike> = {}): KBModelLike => ({
  id,
  type: 'Embedding',
  ...extra,
})

test('优先选择同类型默认模型，没有默认时选择第一条', () => {
  assert.equal(
    pickPreferredKBModelId([chat('chat-a'), chat('chat-b', { is_default: true })], 'KnowledgeQA'),
    'chat-b',
  )
  assert.equal(
    pickPreferredKBModelId([embedding('embedding-a'), embedding('embedding-b')], 'Embedding'),
    'embedding-a',
  )
})

test('普通账号没有可见模型时不阻塞创建，交给后端系统默认值补齐', () => {
  assert.deepEqual(
    getMissingExplicitKBModelKeys({
      models: [],
      config: { llmModelId: '', embeddingModelId: '' },
      needsEmbedding: true,
    }),
    [],
  )
})

test('有可见模型时仍要求用户显式选择必填模型', () => {
  assert.deepEqual(
    getMissingExplicitKBModelKeys({
      models: [chat('chat-a'), embedding('embedding-a')],
      config: { llmModelId: '', embeddingModelId: '' },
      needsEmbedding: true,
    }),
    ['embedding', 'llm'],
  )
})

test('关闭 RAG 索引时不要求 Embedding 模型', () => {
  assert.deepEqual(
    getMissingExplicitKBModelKeys({
      models: [chat('chat-a'), embedding('embedding-a')],
      config: { llmModelId: '', embeddingModelId: '' },
      needsEmbedding: false,
    }),
    ['llm'],
  )
})
