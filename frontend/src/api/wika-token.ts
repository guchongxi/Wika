export const WIKA_TOKEN_SECRET_FIELDS = ['token', 'token_hash', 'created_by_ip'] as const

export type WikaTokenScope =
  | 'knowledge:push'
  | 'knowledge:search'
  | 'knowledge:read'
  | 'suggestion:create'
  | 'mcp:admin'

export interface WikaToken {
  id: number
  name: string
  token?: string
  token_prefix: string
  scopes: WikaTokenScope[]
  expires_at: string
  revoked_at?: string | null
  created_at?: string
  last_used_at?: string | null
}

export interface CreateWikaTokenPayload {
  name: string
  scopes: WikaTokenScope[]
  expires_at: string
}

export interface WikaTokenUsageQuery {
  scope?: 'mine' | 'all'
  owner_user_id?: string
  token_id?: number
  tool_name?: string
  success?: boolean
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export interface WikaTokenUsageToolSummary {
  tool_name: string
  api_method: string
  api_path: string
  success_count: number
  failure_count: number
  last_success_at?: string | null
  last_failure_at?: string | null
  last_status_code?: number
  last_error_code?: string
  last_latency_ms?: number
}

export interface WikaTokenUsageToken {
  id: number
  name: string
  token_prefix: string
  owner_user_id: string
  owner_username?: string
  owner_email?: string
  scopes: WikaTokenScope[]
  status: 'valid' | 'expired' | 'revoked'
  connection_status: 'active' | 'idle' | 'error' | 'expired' | 'revoked' | 'never_used'
  expires_at: string
  revoked_at?: string | null
  created_at?: string
  last_used_at?: string | null
}

export interface WikaTokenUsageSummary {
  total_calls: number
  success_count: number
  failure_count: number
  last_status_code?: number
  last_error_code?: string
  last_success_at?: string | null
  last_failure_at?: string | null
  last_latency_ms?: number
  tools: WikaTokenUsageToolSummary[]
}

export interface WikaTokenUsageItem {
  token: WikaTokenUsageToken
  summary: WikaTokenUsageSummary
}

export interface WikaTokenUsageResponse {
  scope: 'mine' | 'all'
  total: number
  items: WikaTokenUsageItem[]
}

export interface WikaTokenUsageEvent {
  id: number
  token_id: number
  token_name: string
  token_prefix: string
  owner_user_id: string
  owner_username?: string
  tool_name: string
  api_method: string
  api_path: string
  status_code: number
  success: boolean
  error_code?: string
  latency_ms: number
  knowledge_id?: string
  created_at: string
}

export interface WikaTokenUsageEventsResponse {
  scope: 'mine' | 'all'
  total: number
  events: WikaTokenUsageEvent[]
}

const usageQueryOrder: Array<keyof WikaTokenUsageQuery> = [
  'scope',
  'owner_user_id',
  'token_id',
  'tool_name',
  'success',
  'from',
  'to',
  'limit',
  'offset',
]

export function buildWikaTokenUsageQuery(query: WikaTokenUsageQuery = {}): string {
  const params = new URLSearchParams()
  for (const key of usageQueryOrder) {
    const value = query[key]
    if (value === undefined || value === null || value === '') continue
    params.set(key, String(value))
  }
  return params.toString()
}

export async function listWikaTokens(): Promise<WikaToken[]> {
  const { get } = await import('../utils/request')
  const response: any = await get('/api/v1/wika/tokens')
  return response.tokens ?? response.data?.tokens ?? []
}

export async function createWikaToken(payload: CreateWikaTokenPayload): Promise<WikaToken> {
  const { post } = await import('../utils/request')
  return await post('/api/v1/wika/tokens', payload) as WikaToken
}

export async function revokeWikaToken(id: number): Promise<void> {
  const { del } = await import('../utils/request')
  await del(`/api/v1/wika/tokens/${id}`)
}

export async function listWikaTokenUsage(query: WikaTokenUsageQuery = {}): Promise<WikaTokenUsageResponse> {
  const { get } = await import('../utils/request')
  const qs = buildWikaTokenUsageQuery(query)
  const response: any = await get(`/api/v1/wika/tokens/usage${qs ? `?${qs}` : ''}`)
  return response as WikaTokenUsageResponse
}

export async function listWikaTokenUsageEvents(query: WikaTokenUsageQuery = {}): Promise<WikaTokenUsageEventsResponse> {
  const { get } = await import('../utils/request')
  const qs = buildWikaTokenUsageQuery(query)
  const response: any = await get(`/api/v1/wika/tokens/usage/events${qs ? `?${qs}` : ''}`)
  return response as WikaTokenUsageEventsResponse
}
