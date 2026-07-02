import assert from 'node:assert/strict'
import test from 'node:test'

import {
  WIKA_TOKEN_SECRET_FIELDS,
  buildWikaTokenUsageQuery,
} from './wika-token.ts'

test('Wika token usage 查询参数按稳定顺序序列化', () => {
  assert.equal(
    buildWikaTokenUsageQuery({
      scope: 'all',
      owner_user_id: 'u-other',
      token_id: 12,
      tool_name: 'push_knowledge',
      success: false,
      from: '2026-07-01',
      to: '2026-07-02',
      limit: 10,
      offset: 5,
    }),
    'scope=all&owner_user_id=u-other&token_id=12&tool_name=push_knowledge&success=false&from=2026-07-01&to=2026-07-02&limit=10&offset=5',
  )
})

test('Wika token API 模块显式标记禁止进入响应的 secret 字段', () => {
  assert.deepEqual(WIKA_TOKEN_SECRET_FIELDS, ['token', 'token_hash', 'created_by_ip'])
})
