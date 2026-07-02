<template>
  <div class="wika-token-settings">
    <div class="section-header">
      <h2>{{ $t('settings.wikaTokens.title') }}</h2>
      <p class="section-description">{{ $t('settings.wikaTokens.description') }}</p>
    </div>

    <t-tabs v-model="activeTab" placement="top" class="wika-tabs">
      <t-tab-panel value="guide" :label="$t('settings.wikaTokens.guide')">
        <p class="guide-note">使用云端统一 MCP 地址接入，无需在本地安装或启动 Wika 项目。默认开放知识生产工具，管理工具需管理员 PAT 和管理模式请求头。</p>
        <div class="guide-grid">
          <section class="guide-section">
            <h3>远程 MCP 地址</h3>
            <pre><code>{{ remoteMcpUrl }}</code></pre>
          </section>
          <section class="guide-section">
            <h3>Remote MCP 配置</h3>
            <pre><code>{
  "mcpServers": {
    "wika": {
      "type": "streamable-http",
      "url": "{{ remoteMcpUrl }}",
      "headers": {
        "Authorization": "Bearer wika_pat_xxx"
      }
    }
  }
}</code></pre>
          </section>
          <section v-if="canViewAll" class="guide-section guide-section--wide">
            <h3>管理员 MCP 配置</h3>
            <pre><code>{
  "mcpServers": {
    "wika-admin": {
      "type": "streamable-http",
      "url": "{{ remoteMcpUrl }}",
      "headers": {
        "Authorization": "Bearer wika_pat_xxx",
        "X-Wika-MCP-Toolset": "admin"
      }
    }
  }
}</code></pre>
          </section>
          <section class="guide-section guide-section--wide">
            <h3>API 调用</h3>
            <pre><code>curl {{ apiBaseUrl }}/wika/knowledge/push \
  -H 'Authorization: Bearer wika_pat_xxx' \
  -H 'Content-Type: application/json' \
  -d '{"title":"排查记录","content":"先看日志，再看指标。","idempotency_key":"demo-001"}'</code></pre>
          </section>
        </div>
      </t-tab-panel>

      <t-tab-panel value="tokens" :label="$t('settings.wikaTokens.myTokens')">
        <div class="toolbar">
          <t-button theme="primary" @click="openCreateDialog">
            <template #icon><t-icon name="add" /></template>
            {{ $t('settings.wikaTokens.createToken') }}
          </t-button>
          <t-button variant="text" :loading="loadingTokens" @click="loadTokens">
            <template #icon><t-icon name="refresh" /></template>
            刷新
          </t-button>
        </div>

        <div v-if="createdPlaintext" class="created-token">
          <div>
            <strong>{{ $t('settings.wikaTokens.createdToken') }}</strong>
            <p>{{ $t('settings.wikaTokens.createdTokenDesc') }}</p>
          </div>
          <div class="created-token__value">
            <code>{{ createdPlaintext }}</code>
            <t-button size="small" variant="text" @click="copyText(createdPlaintext)">
              <t-icon name="file-copy" />
            </t-button>
          </div>
        </div>

        <div v-if="loadingTokens" class="loading-inline">
          <t-loading size="small" />
          <span>加载中...</span>
        </div>
        <table v-else class="usage-table">
          <thead>
            <tr>
              <th>名称</th>
              <th>Prefix</th>
              <th>Scope</th>
              <th>状态</th>
              <th>过期时间</th>
              <th>最近调用</th>
              <th class="table-action">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="token in tokens" :key="token.id">
              <td>{{ token.name }}</td>
              <td><code>{{ token.token_prefix }}</code></td>
              <td>{{ token.scopes.join(', ') }}</td>
              <td>
                <t-tag :theme="token.revoked_at ? 'danger' : 'success'" variant="light">
                  {{ token.revoked_at ? '已撤销' : '有效' }}
                </t-tag>
              </td>
              <td>{{ formatTime(token.expires_at) }}</td>
              <td>{{ formatTime(token.last_used_at) }}</td>
              <td class="table-action">
                <t-button size="small" theme="danger" variant="text" :disabled="Boolean(token.revoked_at)" @click="revokeToken(token.id)">
                  撤销
                </t-button>
              </td>
            </tr>
            <tr v-if="tokens.length === 0">
              <td colspan="7" class="empty-cell">暂无 Token</td>
            </tr>
          </tbody>
        </table>
      </t-tab-panel>

      <t-tab-panel value="usage" :label="$t('settings.wikaTokens.usage')">
        <div class="toolbar usage-toolbar">
          <t-radio-group v-if="canViewAll" v-model="usageScope" variant="default-filled">
            <t-radio-button value="mine">我的</t-radio-button>
            <t-radio-button value="all">租户全部</t-radio-button>
          </t-radio-group>
          <t-input v-if="usageScope === 'all'" v-model="ownerFilter" clearable placeholder="Owner User ID" class="owner-filter" />
          <t-select v-model="toolFilter" clearable placeholder="Tool/API" class="tool-filter">
            <t-option value="push_knowledge" label="push_knowledge" />
            <t-option value="search_knowledge" label="search_knowledge" />
            <t-option value="expand_knowledge_result" label="expand_knowledge_result" />
            <t-option value="get_my_knowledge" label="get_my_knowledge" />
            <t-option value="suggest_to_team" label="suggest_to_team" />
          </t-select>
          <t-button variant="text" :loading="loadingUsage" @click="loadUsage">
            <template #icon><t-icon name="refresh" /></template>
            刷新
          </t-button>
        </div>

        <table class="usage-table">
          <thead>
            <tr>
              <th>Token</th>
              <th>Owner</th>
              <th>连接状态</th>
              <th>调用</th>
              <th>成功</th>
              <th>失败</th>
              <th>最近成功</th>
              <th>最近失败</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in usageItems" :key="item.token.id">
              <td>{{ item.token.name }} <code>{{ item.token.token_prefix }}</code></td>
              <td>{{ item.token.owner_username || item.token.owner_user_id }}</td>
              <td><t-tag variant="light">{{ item.token.connection_status }}</t-tag></td>
              <td>{{ item.summary.total_calls }}</td>
              <td>{{ item.summary.success_count }}</td>
              <td>{{ item.summary.failure_count }}</td>
              <td>{{ formatTime(item.summary.last_success_at) }}</td>
              <td>{{ formatTime(item.summary.last_failure_at) }}</td>
            </tr>
            <tr v-if="usageItems.length === 0">
              <td colspan="8" class="empty-cell">暂无调用统计</td>
            </tr>
          </tbody>
        </table>

        <h3 class="sub-title">最近调用</h3>
        <table class="usage-table">
          <thead>
            <tr>
              <th>时间</th>
              <th>Tool</th>
              <th>Token</th>
              <th>Owner</th>
              <th>状态码</th>
              <th>耗时</th>
              <th>Knowledge ID</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="event in usageEvents" :key="event.id">
              <td>{{ formatTime(event.created_at) }}</td>
              <td>{{ event.tool_name }}</td>
              <td>{{ event.token_name }} <code>{{ event.token_prefix }}</code></td>
              <td>{{ event.owner_username || event.owner_user_id }}</td>
              <td>{{ event.status_code }}</td>
              <td>{{ event.latency_ms }}ms</td>
              <td><code>{{ event.knowledge_id || '-' }}</code></td>
            </tr>
            <tr v-if="usageEvents.length === 0">
              <td colspan="7" class="empty-cell">暂无调用事件</td>
            </tr>
          </tbody>
        </table>
      </t-tab-panel>
    </t-tabs>

    <t-dialog
      v-model:visible="createDialogVisible"
      header="创建 Wika PAT"
      :confirm-loading="creating"
      @confirm="createToken"
    >
      <div class="token-form">
        <label>名称</label>
        <t-input v-model="createForm.name" placeholder="Claude Code" />
        <label>Scope</label>
        <t-checkbox-group v-model="createForm.scopes">
          <t-checkbox value="knowledge:push">knowledge:push</t-checkbox>
          <t-checkbox value="knowledge:read">knowledge:read</t-checkbox>
          <t-checkbox value="knowledge:search">knowledge:search</t-checkbox>
          <t-checkbox value="suggestion:create">suggestion:create</t-checkbox>
          <t-checkbox v-if="canViewAll" value="mcp:admin">mcp:admin</t-checkbox>
        </t-checkbox-group>
        <label>过期时间</label>
        <input v-model="expiresLocal" type="datetime-local" class="native-input" />
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useAuthStore } from '@/stores/auth'
import { getApiBaseUrl } from '@/utils/api-base'
import {
  createWikaToken,
  listWikaTokens,
  listWikaTokenUsage,
  listWikaTokenUsageEvents,
  revokeWikaToken,
  type WikaToken,
  type WikaTokenScope,
  type WikaTokenUsageEvent,
  type WikaTokenUsageItem,
} from '@/api/wika-token'

const authStore = useAuthStore()
const activeTab = ref('guide')
const tokens = ref<WikaToken[]>([])
const usageItems = ref<WikaTokenUsageItem[]>([])
const usageEvents = ref<WikaTokenUsageEvent[]>([])
const loadingTokens = ref(false)
const loadingUsage = ref(false)
const creating = ref(false)
const createDialogVisible = ref(false)
const createdPlaintext = ref('')
const usageScope = ref<'mine' | 'all'>('mine')
const ownerFilter = ref('')
const toolFilter = ref('')
const expiresLocal = ref('')

const createForm = reactive<{ name: string; scopes: WikaTokenScope[] }>({
  name: 'Claude Code',
  scopes: ['knowledge:push', 'knowledge:read', 'knowledge:search'],
})

const canViewAll = computed(() => authStore.canAccessAllTenants || authStore.hasRole('admin'))

const apiBaseUrl = computed(() => {
  const configured = getApiBaseUrl().trim().replace(/\/$/, '')
  const origin = typeof window !== 'undefined' && window.location.origin !== 'null' ? window.location.origin : ''
  return `${configured || origin}/api/v1`
})

const remoteMcpUrl = computed(() => {
  try {
    const url = new URL(apiBaseUrl.value)
    if ((url.hostname === 'localhost' || url.hostname === '127.0.0.1') && url.port === '8080') {
      url.port = '8082'
    }
    url.pathname = '/mcp'
    url.search = ''
    url.hash = ''
    return url.toString()
  } catch {
    return apiBaseUrl.value.replace(/\/api\/v1$/, '/mcp')
  }
})

function defaultExpiresLocal() {
  const d = new Date()
  d.setDate(d.getDate() + 30)
  d.setSeconds(0, 0)
  return d.toISOString().slice(0, 16)
}

function formatTime(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString()
}

function openCreateDialog() {
  createdPlaintext.value = ''
  createForm.name = 'Claude Code'
  createForm.scopes = ['knowledge:push', 'knowledge:read', 'knowledge:search']
  expiresLocal.value = defaultExpiresLocal()
  createDialogVisible.value = true
}

async function loadTokens() {
  loadingTokens.value = true
  try {
    tokens.value = await listWikaTokens()
  } catch (error: any) {
    MessagePlugin.error(error?.message || '加载 Token 失败')
  } finally {
    loadingTokens.value = false
  }
}

async function createToken() {
  if (!createForm.name.trim() || createForm.scopes.length === 0 || !expiresLocal.value) {
    MessagePlugin.warning('请填写名称、Scope 和过期时间')
    return
  }
  creating.value = true
  try {
    const created = await createWikaToken({
      name: createForm.name.trim(),
      scopes: createForm.scopes,
      expires_at: new Date(expiresLocal.value).toISOString(),
    })
    createdPlaintext.value = created.token || ''
    createDialogVisible.value = false
    await loadTokens()
  } catch (error: any) {
    MessagePlugin.error(error?.message || '创建 Token 失败')
  } finally {
    creating.value = false
  }
}

async function revokeToken(id: number) {
  try {
    await revokeWikaToken(id)
    await loadTokens()
    await loadUsage()
  } catch (error: any) {
    MessagePlugin.error(error?.message || '撤销 Token 失败')
  }
}

async function loadUsage() {
  loadingUsage.value = true
  try {
    const query = {
      scope: usageScope.value,
      owner_user_id: usageScope.value === 'all' ? ownerFilter.value.trim() || undefined : undefined,
      tool_name: toolFilter.value || undefined,
      limit: 100,
    }
    const [usage, events] = await Promise.all([
      listWikaTokenUsage(query),
      listWikaTokenUsageEvents(query),
    ])
    usageItems.value = usage.items || []
    usageEvents.value = events.events || []
  } catch (error: any) {
    MessagePlugin.error(error?.message || '加载调用统计失败')
  } finally {
    loadingUsage.value = false
  }
}

async function copyText(text: string) {
  await navigator.clipboard?.writeText(text)
  MessagePlugin.success('已复制')
}

watch([usageScope, toolFilter], () => {
  loadUsage()
})

onMounted(() => {
  expiresLocal.value = defaultExpiresLocal()
  loadTokens()
  loadUsage()
})
</script>

<style scoped lang="less">
.wika-token-settings {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.section-header {
  margin-bottom: 8px;

  h2 {
    margin: 0 0 8px;
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
}

.section-description {
  margin: 0;
  color: var(--td-text-color-secondary);
  font-size: 14px;
}

.wika-tabs {
  min-height: 0;

  :deep(.t-tabs__content) {
    margin-top: 16px;
  }
}

.guide-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.guide-note {
  margin: 0 0 12px;
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.guide-section {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  padding: 14px;
  min-width: 0;

  h3 {
    margin: 0 0 10px;
    font-size: 14px;
    font-weight: 600;
  }

  pre {
    margin: 0;
    overflow-x: auto;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 6px;
    padding: 10px;
    font-size: 12px;
    line-height: 1.6;
  }
}

.guide-section--wide {
  grid-column: 1 / -1;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}

.owner-filter,
.tool-filter {
  width: 220px;
}

.created-token {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  align-items: center;
  border: 1px solid var(--td-success-color-3);
  border-radius: 8px;
  padding: 12px;
  margin-bottom: 12px;
  background: var(--td-success-color-1);

  p {
    margin: 4px 0 0;
    color: var(--td-text-color-secondary);
  }
}

.created-token__value {
  display: flex;
  align-items: center;
  gap: 6px;
  max-width: 420px;

  code {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.loading-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--td-text-color-secondary);
  padding: 16px 0;
}

.usage-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;

  th,
  td {
    border-bottom: 1px solid var(--td-component-stroke);
    padding: 10px 8px;
    text-align: left;
    vertical-align: middle;
  }

  th {
    color: var(--td-text-color-secondary);
    font-weight: 500;
    background: var(--td-bg-color-secondarycontainer);
  }

  code {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 12px;
  }
}

.table-action {
  text-align: right;
  white-space: nowrap;
}

.empty-cell {
  text-align: center;
  color: var(--td-text-color-placeholder);
}

.sub-title {
  margin: 18px 0 10px;
  font-size: 15px;
  font-weight: 600;
}

.token-form {
  display: grid;
  gap: 10px;

  label {
    color: var(--td-text-color-secondary);
    font-size: 13px;
  }
}

.native-input {
  width: 100%;
  height: 32px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  padding: 0 10px;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-container);
}

@media (max-width: 720px) {
  .guide-grid {
    grid-template-columns: 1fr;
  }

  .created-token {
    grid-template-columns: 1fr;
  }
}
</style>
