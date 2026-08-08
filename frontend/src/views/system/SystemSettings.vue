<template>
  <div class="system-settings">
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="visible" class="system-settings-overlay" @click.self="handleClose">
          <div class="system-settings-modal">
            <button class="system-settings-close" type="button" @click="handleClose" :aria-label="t('general.close')">
              <t-icon name="close" />
            </button>

            <div class="system-settings-container">
              <aside class="system-settings-sidebar">
                <div class="system-settings-sidebar-header">
                  <h2>{{ t('system.globalSettings.title') }}</h2>
                  <p>{{ t('system.globalSettings.description') }}</p>
                </div>

                <nav class="system-settings-nav" aria-label="System settings">
                  <button
                    v-for="tab in systemSettingTabs"
                    :key="tab.key"
                    type="button"
                    class="system-settings-nav-item"
                    :class="{ active: activeTab === tab.key }"
                    @click="setActiveTab(tab.key)"
                  >
                    <t-icon :name="tab.icon" class="system-settings-nav-icon" />
                    <span>{{ t(tab.labelKey) }}</span>
                  </button>
                </nav>
              </aside>

              <main class="system-settings-content">
                <div class="system-settings-content-header">
                  <div>
                    <h2>{{ t(currentTab.labelKey) }}</h2>
                    <p>{{ t(currentTab.descriptionKey) }}</p>
                  </div>
                  <t-button variant="text" size="small" class="header-audit-btn" @click="openAuditDrawer">
                    <template #icon><t-icon name="history" /></template>
                    {{ t('system.globalSettings.audit.tabLabel') }}
                  </t-button>
                </div>

                <div class="system-settings-content-body">
                  <template v-if="activeTab === 'models-services'">
                    <div class="model-service-intro">
                      <div
                        v-for="section in modelServiceSections"
                        :key="section.key"
                        class="model-service-item"
                        :class="{ active: activeServiceSection === section.key }"
                        :data-service-section="section.key"
                        @click="setActiveServiceSection(section.key)"
                      >
                        <t-icon :name="section.icon" class="model-service-icon" />
                        <div>
                          <div class="model-service-title">{{ t(section.labelKey) }}</div>
                          <p>{{ t(section.descriptionKey) }}</p>
                        </div>
                      </div>
                    </div>
                    <div class="embedded-model-settings">
                      <ModelSettings mode="system" :initial-type="activeModelSettingsType" />
                    </div>
                  </template>

                  <template v-else-if="activeTab === 'system-admins'">
                    <div class="settings-group system-settings-group">
                      <div class="setting-group-header">
                        <h3>{{ t('system.globalSettings.groups.admins.title') }}</h3>
                        <p>{{ t('system.globalSettings.groups.admins.description') }}</p>
                      </div>
                      <div class="setting-row">
                        <div class="setting-info">
                          <label class="setting-label">
                            <span>{{ t('system.globalSettings.admins.label') }}</span>
                          </label>
                          <p class="desc">{{ t('system.globalSettings.admins.description') }}</p>
                        </div>
                        <div class="setting-control">
                          <div class="setting-control-row">
                            <t-popconfirm
                              v-model:visible="adminPopconfirm.visible"
                              :content="adminPopconfirm.content"
                              :theme="adminPopconfirm.theme"
                              :confirm-btn="adminPopconfirm.confirmBtn"
                              :cancel-btn="t('system.globalSettings.confirm.cancelBtn')"
                              :popup-props="PROGRAMMATIC_POPCONFIRM_PROPS"
                              placement="left"
                              @confirm="adminPopconfirm.finish(true)"
                              @cancel="adminPopconfirm.finish(false)"
                              @visible-change="adminPopconfirm.onVisibleChange"
                            >
                              <div class="setting-control-anchor">
                                <t-tag-input
                                  v-model="adminEmails"
                                  :placeholder="t('system.globalSettings.admins.placeholder')"
                                  :disabled="adminBusy"
                                  class="setting-input setting-input--wide"
                                  clearable
                                  @change="onAdminsChange"
                                />
                              </div>
                            </t-popconfirm>
                            <t-loading v-if="adminBusy" size="small" class="setting-saving" />
                          </div>
                        </div>
                      </div>

                      <div class="setting-row">
                        <div class="setting-info">
                          <label class="setting-label">
                            <span>{{ t('system.globalSettings.audit.tabLabel') }}</span>
                          </label>
                          <p class="desc">{{ t('system.globalSettings.audit.description') }}</p>
                        </div>
                        <div class="setting-control">
                          <div class="setting-control-row">
                            <t-button variant="outline" size="small" @click="openAuditDrawer">
                              <template #icon><t-icon name="history" /></template>
                              {{ t('system.globalSettings.admins.auditButton') }}
                            </t-button>
                          </div>
                        </div>
                      </div>
                    </div>
                  </template>

                  <template v-else>
                    <div class="priority-hint">
                      <div class="priority-hint-header">
                        <t-icon name="info-circle-filled" class="priority-hint-icon" />
                        <span class="priority-hint-title">
                          {{ t('system.globalSettings.priorityHint.title') }}
                        </span>
                      </div>
                      <ul class="priority-hint-list">
                        <li>{{ t('system.globalSettings.priorityHint.tier1') }}</li>
                        <li>{{ t('system.globalSettings.priorityHint.tier2') }}</li>
                        <li>{{ t('system.globalSettings.priorityHint.tier3') }}</li>
                      </ul>
                    </div>

                    <div v-if="loading && settings.length === 0" class="loading-state">
                      <t-loading :text="t('system.globalSettings.loading')" />
                    </div>

                    <div v-else-if="settings.length === 0" class="empty-state">
                      <t-icon name="info-circle" size="24px" />
                      <span>{{ t('system.globalSettings.empty') }}</span>
                    </div>

                    <div v-else>
                      <div
                        v-for="group in settingGroupsForActiveTab"
                        :key="group.key"
                        class="settings-group system-settings-group"
                        :data-setting-group="group.key"
                      >
                        <div class="setting-group-header">
                          <h3>{{ t(group.titleKey) }}</h3>
                          <p>{{ t(group.descriptionKey) }}</p>
                        </div>

                        <div v-if="groupItems(group).length === 0" class="empty-state empty-state--group">
                          <t-icon name="info-circle" size="20px" />
                          <span>{{ t('system.globalSettings.emptyGroup') }}</span>
                        </div>

                        <div
                          v-for="item in groupItems(group)"
                          :key="item.key"
                          class="setting-row"
                        >
                          <div class="setting-info">
                            <label class="setting-label">
                              <span>{{ keyLabel(item.key) }}</span>
                              <t-tag
                                v-if="item.requires_restart"
                                theme="warning"
                                variant="light"
                                size="small"
                                class="setting-badge"
                              >{{ t('system.globalSettings.badgeRequiresRestart') }}</t-tag>
                              <t-tag
                                v-if="item.is_secret"
                                theme="primary"
                                variant="light"
                                size="small"
                                class="setting-badge"
                              >{{ t('system.globalSettings.badgeSecret') }}</t-tag>
                              <t-tag
                                v-if="hasOverride(item)"
                                theme="success"
                                variant="light"
                                size="small"
                                class="setting-badge"
                                :title="t('system.globalSettings.badgeOverrideTooltip')"
                              >{{ t('system.globalSettings.badgeOverride') }}</t-tag>
                            </label>
                            <p v-if="settingDescription(item)" class="desc">{{ settingDescription(item) }}</p>
                            <div v-if="modifiedMeta(item)" class="setting-meta">
                              {{ t('system.globalSettings.modifiedAt', { value: modifiedMeta(item) }) }}
                            </div>
                          </div>

                          <div class="setting-control">
                            <div class="setting-control-row">
                              <t-popconfirm
                                v-if="hasEnum(item) && isHighRiskKey(item.key)"
                                v-model:visible="highRiskPopconfirm.visible"
                                :content="highRiskPopconfirm.content"
                                :theme="highRiskPopconfirm.theme"
                                :confirm-btn="highRiskPopconfirm.confirmBtn"
                                :cancel-btn="t('system.globalSettings.confirm.cancelBtn')"
                                :popup-props="PROGRAMMATIC_POPCONFIRM_PROPS"
                                placement="left"
                                @confirm="highRiskPopconfirm.finish(true)"
                                @cancel="highRiskPopconfirm.finish(false)"
                                @visible-change="highRiskPopconfirm.onVisibleChange"
                              >
                                <div class="setting-control-anchor">
                                  <t-select
                                    v-model="editValues[item.key]"
                                    :options="enumOptions(item)"
                                    :disabled="savingKey === item.key"
                                    class="setting-input"
                                    @change="onHighRiskSelectChange(item)"
                                  />
                                </div>
                              </t-popconfirm>
                              <t-select
                                v-else-if="hasEnum(item)"
                                v-model="editValues[item.key]"
                                :options="enumOptions(item)"
                                :disabled="savingKey === item.key"
                                class="setting-input"
                                @change="onChange(item)"
                              />
                              <ModelSelector
                                v-else-if="isModelSetting(item.key)"
                                :model-type="modelSettingType(item.key)"
                                :selected-model-id="String(editValues[item.key] || '')"
                                :placeholder="t('model.selectModelPlaceholder')"
                                :disabled="savingKey === item.key"
                                class="setting-input setting-input--wide"
                                @update:selected-model-id="(value) => onModelSettingChange(item, value)"
                                @add-model="openModelSettings(item.key)"
                              />
                              <t-switch
                                v-else-if="item.value_type === 'bool'"
                                v-model="editValues[item.key]"
                                :disabled="savingKey === item.key"
                                @change="onChange(item)"
                              />
                              <t-input-number
                                v-else-if="item.value_type === 'int'"
                                v-model="editValues[item.key]"
                                :placeholder="placeholderFor(item)"
                                :disabled="savingKey === item.key"
                                theme="normal"
                                :step="1"
                                :min="0"
                                class="setting-input"
                                @blur="onChange(item)"
                              />
                              <t-popconfirm
                                v-else-if="item.value_type === 'string_list' && item.key === 'ssrf.whitelist'"
                                v-model:visible="ssrfPopconfirm.visible"
                                :content="ssrfPopconfirm.content"
                                :theme="ssrfPopconfirm.theme"
                                :confirm-btn="ssrfPopconfirm.confirmBtn"
                                :cancel-btn="t('system.globalSettings.confirm.cancelBtn')"
                                :popup-props="PROGRAMMATIC_POPCONFIRM_PROPS"
                                placement="left"
                                @confirm="ssrfPopconfirm.finish(true)"
                                @cancel="ssrfPopconfirm.finish(false)"
                                @visible-change="ssrfPopconfirm.onVisibleChange"
                              >
                                <div class="setting-control-anchor">
                                  <t-tag-input
                                    :key="`ssrf-tag-${ssrfTagInputKey()}`"
                                    :model-value="ssrfWhitelistModelValue()"
                                    :placeholder="emptyListPlaceholder"
                                    :disabled="savingKey === item.key"
                                    class="setting-input setting-input--wide"
                                    clearable
                                    @update:model-value="onSsrfWhitelistModelUpdate"
                                  />
                                </div>
                              </t-popconfirm>
                              <t-input
                                v-else
                                v-model="editValues[item.key]"
                                :placeholder="placeholderFor(item)"
                                :disabled="savingKey === item.key"
                                class="setting-input"
                                clearable
                                @blur="onChange(item)"
                              />

                              <t-loading
                                v-if="savingKey === item.key"
                                size="small"
                                class="setting-saving"
                              />
                            </div>

                            <div
                              v-if="hasOverride(item) || hasBulkAction(item) || hasRelatedAction(item)"
                              class="setting-control-actions"
                            >
                              <t-button
                                v-if="hasRelatedAction(item)"
                                variant="text"
                                size="small"
                                class="setting-related-btn"
                                @click="openRelatedArea(item)"
                              >
                                <template #icon><t-icon :name="relatedActionIcon(item)" /></template>
                                {{ relatedActionLabel(item) }}
                              </t-button>

                              <t-popconfirm
                                v-if="hasBulkAction(item)"
                                :content="bulkActionConfirmBody(item)"
                                :confirm-btn="{ content: t('system.globalSettings.bulkApply.confirmBtn'), theme: 'primary' }"
                                :cancel-btn="{ content: t('system.globalSettings.confirm.cancelBtn') }"
                                placement="left"
                                @confirm="runBulkAction(item)"
                              >
                                <t-button
                                  variant="text"
                                  size="small"
                                  :disabled="savingKey === item.key || isDirty(item)"
                                  :title="t('system.globalSettings.bulkApply.tooltip')"
                                  class="setting-bulk-btn"
                                >
                                  <template #icon><t-icon name="usergroup" /></template>
                                  {{ t('system.globalSettings.bulkApply.label') }}
                                </t-button>
                              </t-popconfirm>

                              <t-popconfirm
                                v-if="hasOverride(item)"
                                :content="t('system.globalSettings.reset.confirmBody', { label: keyLabel(item.key) })"
                                :confirm-btn="{ content: t('system.globalSettings.reset.confirmBtn'), theme: 'warning' }"
                                :cancel-btn="{ content: t('system.globalSettings.confirm.cancelBtn') }"
                                placement="left"
                                @confirm="resetSetting(item)"
                              >
                                <t-button
                                  variant="text"
                                  size="small"
                                  :disabled="savingKey === item.key"
                                  :title="t('system.globalSettings.reset.tooltip')"
                                  class="setting-reset-btn"
                                >
                                  <template #icon><t-icon name="refresh" /></template>
                                  {{ t('system.globalSettings.reset.label') }}
                                </t-button>
                              </t-popconfirm>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  </template>
                </div>
              </main>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Platform audit-log drawer. Lazy-loaded on first open; closing
         and reopening doesn't re-fetch (refresh is explicit via the
         button inside the drawer). Backend route is SystemAdmin-gated,
         and this whole view is too, so we don't bother with a role
         check — any visitor here is eligible to read the feed. -->
    <t-drawer
      v-model:visible="auditDrawerVisible"
      :header="t('system.globalSettings.audit.tabLabel')"
      drawer-class-name="system-settings-audit-drawer"
      size="880px"
      :footer="false"
      placement="right"
      destroy-on-close
    >
      <div class="audit-drawer-inner audit-panel audit-panel--drawer">
        <div class="audit-header">
          <span class="audit-desc">{{ t('system.globalSettings.audit.description') }}</span>
          <t-button
            variant="text"
            size="small"
            class="audit-refresh-btn"
            :loading="auditLoading"
            :disabled="auditLoading"
            @click="reloadAuditLog"
          >
            <template #icon><t-icon name="refresh" /></template>
            {{ t('system.globalSettings.audit.refresh') }}
          </t-button>
        </div>

        <div class="audit-drawer-fill">
          <div v-if="auditError" class="audit-drawer-branch audit-drawer-branch--error">
            <div class="error-inline">
              <t-alert theme="error" :message="auditError">
                <template #operation>
                  <t-button size="small" @click="reloadAuditLog">
                    {{ t('system.globalSettings.audit.retry') }}
                  </t-button>
                </template>
              </t-alert>
            </div>
          </div>

          <div
            v-else-if="!auditLoading && auditEntries.length === 0"
            class="audit-drawer-branch audit-drawer-branch--empty empty-state empty-state--audit"
          >
            <t-empty :description="t('system.globalSettings.audit.empty')" />
          </div>

          <div v-else class="audit-scroll-area narrow-scrollbar audit-drawer-branch" ref="auditScrollRoot">
            <div class="data-table-shell audit-table-shell">
              <t-table
                row-key="id"
                :data="auditEntries"
                :columns="auditColumns"
                size="medium"
                hover
                expand-on-row-click
                :expanded-row-keys="auditExpandedRowKeys"
                @expand-change="onAuditExpandChange"
              >
                <template #created_at="{ row }">
                  <div class="audit-time">
                    <span class="audit-time-date">{{ formatAuditDatePart(row.created_at) }}</span>
                    <span class="audit-time-clock">{{ formatAuditTimePart(row.created_at) }}</span>
                  </div>
                </template>
                <template #actor="{ row }">
                  <div class="audit-actor">
                    <span class="audit-actor-name">
                      {{ row.actor_user_id ? auditActorLabel(row.actor_user_id) :
                        t('system.globalSettings.audit.systemActor') }}
                    </span>
                    <span v-if="row.actor_role" class="audit-actor-role">
                      {{ auditActorRoleLabel(row.actor_role) }}
                    </span>
                  </div>
                </template>
                <template #action="{ row }">
                  <t-tag :theme="auditActionTheme(row.action)" size="small" variant="light-outline">
                    {{ formatAuditAction(row.action) }}
                  </t-tag>
                </template>
                <template #target="{ row }">
                  <div class="audit-target">
                    <span v-if="auditTargetKey(row)" class="audit-target-key">{{ auditTargetKey(row) }}</span>
                    <span v-if="auditTargetDiff(row)" class="audit-target-diff">{{ auditTargetDiff(row) }}</span>
                    <span v-else-if="!auditTargetKey(row)" class="audit-target-empty">—</span>
                  </div>
                </template>
                <template #outcome="{ row }">
                  <t-tag :theme="auditOutcomeTheme(row.outcome)" size="small" variant="light">
                    {{ t('system.globalSettings.audit.outcome.' + row.outcome) }}
                  </t-tag>
                </template>
                <template #expandedRow="{ row }">
                  <div class="audit-expanded">
                    <div class="audit-expanded-grid">
                      <div class="audit-expanded-cell">
                        <span class="audit-expanded-label">{{ t('system.globalSettings.audit.expanded.actorId') }}</span>
                        <span class="audit-expanded-value mono">{{ row.actor_user_id || '—' }}</span>
                      </div>
                      <div v-if="row.target_user_id" class="audit-expanded-cell">
                        <span class="audit-expanded-label">{{ t('system.globalSettings.audit.expanded.targetUserId') }}</span>
                        <span class="audit-expanded-value mono">{{ row.target_user_id }}</span>
                      </div>
                      <div v-if="row.target_type" class="audit-expanded-cell">
                        <span class="audit-expanded-label">{{ t('system.globalSettings.audit.expanded.targetType') }}</span>
                        <span class="audit-expanded-value mono">{{ row.target_type }}</span>
                      </div>
                      <div v-if="row.target_id" class="audit-expanded-cell">
                        <span class="audit-expanded-label">{{ t('system.globalSettings.audit.expanded.targetId') }}</span>
                        <span class="audit-expanded-value mono">{{ row.target_id }}</span>
                      </div>
                    </div>
                    <div class="audit-expanded-details">
                      <span class="audit-expanded-label">{{ t('system.globalSettings.audit.expanded.details') }}</span>
                      <pre class="audit-expanded-json mono">{{ auditDetailsJSON(row) }}</pre>
                    </div>
                  </div>
                </template>
              </t-table>
            </div>

            <div ref="auditLoadSentinelEl" class="audit-load-sentinel" aria-hidden="true" />

            <div v-if="auditLoading && auditEntries.length > 0" class="audit-loading-more">
              <t-loading size="small" />
              <span>{{ t('system.globalSettings.audit.loading') }}</span>
            </div>

            <p v-if="!auditHasMore && auditEntries.length > 0 && !auditLoading" class="audit-end-hint">
              {{ t('system.globalSettings.audit.end') }}
            </p>
          </div>
        </div>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, computed, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listSystemSettings,
  updateSystemSetting,
  resetSystemSetting,
  applyDefaultStorageQuotaToAllTenants,
  listSystemAdmins,
  promoteUserToSystemAdmin,
  revokeSystemAdmin,
  listSystemAuditLog,
  type SystemSettingItem,
  type AuditLog,
  type AuditAction,
  type AuditOutcome,
} from '@/api/system'
import { useAuthStore } from '@/stores/auth'
import { useUIStore } from '@/stores/ui'
import ModelSelector from '@/components/ModelSelector.vue'
import ModelSettings from '@/views/settings/ModelSettings.vue'

const authStore = useAuthStore()
const uiStore = useUIStore()
const currentUserId = computed(() => authStore.currentUserId)

const { t, tm, te, locale } = useI18n()

type SystemTabKey =
  | 'platform-runtime'
  | 'account-tenant'
  | 'kb-defaults'
  | 'models-services'
  | 'security-network'
  | 'governance'
  | 'system-admins'

type ServiceSectionKey = 'chat' | 'embedding' | 'vllm' | 'asr' | 'storage' | 'vectorstore' | 'parser'
type ModelSettingsInitialType = 'all' | 'chat' | 'embedding' | 'vllm' | 'asr'

type SystemSettingTab = {
  key: SystemTabKey
  icon: string
  labelKey: string
  descriptionKey: string
}

type SettingGroupDefinition = {
  key: string
  tab: SystemTabKey
  titleKey: string
  descriptionKey: string
  settingKeys: string[]
}

const systemSettingTabs: SystemSettingTab[] = [
  {
    key: 'platform-runtime',
    icon: 'server',
    labelKey: 'system.globalSettings.tabs.platformRuntime.label',
    descriptionKey: 'system.globalSettings.tabs.platformRuntime.description',
  },
  {
    key: 'account-tenant',
    icon: 'user-circle',
    labelKey: 'system.globalSettings.tabs.accountTenant.label',
    descriptionKey: 'system.globalSettings.tabs.accountTenant.description',
  },
  {
    key: 'kb-defaults',
    icon: 'book',
    labelKey: 'system.globalSettings.tabs.kbDefaults.label',
    descriptionKey: 'system.globalSettings.tabs.kbDefaults.description',
  },
  {
    key: 'models-services',
    icon: 'control-platform',
    labelKey: 'system.globalSettings.tabs.modelsServices.label',
    descriptionKey: 'system.globalSettings.tabs.modelsServices.description',
  },
  {
    key: 'security-network',
    icon: 'secured',
    labelKey: 'system.globalSettings.tabs.securityNetwork.label',
    descriptionKey: 'system.globalSettings.tabs.securityNetwork.description',
  },
  {
    key: 'governance',
    icon: 'chart-bubble',
    labelKey: 'system.globalSettings.tabs.governance.label',
    descriptionKey: 'system.globalSettings.tabs.governance.description',
  },
  {
    key: 'system-admins',
    icon: 'usergroup',
    labelKey: 'system.globalSettings.tabs.systemAdmins.label',
    descriptionKey: 'system.globalSettings.tabs.systemAdmins.description',
  },
]

const SYSTEM_SETTING_GROUPS: SettingGroupDefinition[] = [
  {
    key: 'runtime-worker',
    tab: 'platform-runtime',
    titleKey: 'system.globalSettings.groups.runtimeWorker.title',
    descriptionKey: 'system.globalSettings.groups.runtimeWorker.description',
    settingKeys: ['asynq.concurrency'],
  },
  {
    key: 'account-registration',
    tab: 'account-tenant',
    titleKey: 'system.globalSettings.groups.accountRegistration.title',
    descriptionKey: 'system.globalSettings.groups.accountRegistration.description',
    settingKeys: ['auth.registration_mode'],
  },
  {
    key: 'tenant-defaults',
    tab: 'account-tenant',
    titleKey: 'system.globalSettings.groups.tenantDefaults.title',
    descriptionKey: 'system.globalSettings.groups.tenantDefaults.description',
    settingKeys: ['tenant.max_owned_per_user', 'tenant.default_storage_quota_gb'],
  },
  {
    key: 'kb-models',
    tab: 'kb-defaults',
    titleKey: 'system.globalSettings.groups.kbModels.title',
    descriptionKey: 'system.globalSettings.groups.kbModels.description',
    settingKeys: [
      'kb.default_llm_model_id',
      'kb.default_embedding_model_id',
      'kb.default_vlm_model_id',
      'kb.default_asr_model_id',
    ],
  },
  {
    key: 'kb-storage',
    tab: 'kb-defaults',
    titleKey: 'system.globalSettings.groups.kbStorage.title',
    descriptionKey: 'system.globalSettings.groups.kbStorage.description',
    settingKeys: ['kb.default_storage_provider'],
  },
  {
    key: 'kb-index',
    tab: 'kb-defaults',
    titleKey: 'system.globalSettings.groups.kbIndex.title',
    descriptionKey: 'system.globalSettings.groups.kbIndex.description',
    settingKeys: [
      'kb.default_index_vector_enabled',
      'kb.default_index_keyword_enabled',
      'kb.default_index_wiki_enabled',
      'kb.default_index_graph_enabled',
    ],
  },
  {
    key: 'kb-chunking',
    tab: 'kb-defaults',
    titleKey: 'system.globalSettings.groups.kbChunking.title',
    descriptionKey: 'system.globalSettings.groups.kbChunking.description',
    settingKeys: [
      'kb.default_chunk_size',
      'kb.default_chunk_overlap',
      'kb.default_chunk_separators',
      'kb.default_parent_child_enabled',
      'kb.default_parent_chunk_size',
      'kb.default_child_chunk_size',
    ],
  },
  {
    key: 'kb-multimodal',
    tab: 'kb-defaults',
    titleKey: 'system.globalSettings.groups.kbMultimodal.title',
    descriptionKey: 'system.globalSettings.groups.kbMultimodal.description',
    settingKeys: ['kb.default_vlm_enabled', 'kb.default_asr_enabled'],
  },
  {
    key: 'kb-production',
    tab: 'kb-defaults',
    titleKey: 'system.globalSettings.groups.kbProduction.title',
    descriptionKey: 'system.globalSettings.groups.kbProduction.description',
    settingKeys: ['kb.default_question_generation_enabled', 'kb.default_question_generation_count'],
  },
  {
    key: 'network-ssrf',
    tab: 'security-network',
    titleKey: 'system.globalSettings.groups.networkSsrf.title',
    descriptionKey: 'system.globalSettings.groups.networkSsrf.description',
    settingKeys: ['ssrf.whitelist'],
  },
  {
    key: 'governance-switches',
    tab: 'governance',
    titleKey: 'system.globalSettings.groups.governanceSwitches.title',
    descriptionKey: 'system.globalSettings.groups.governanceSwitches.description',
    settingKeys: [
      'wika.governance.conflict.enabled',
      'wika.governance.version.enabled',
      'wika.governance.url_refresh.enabled',
      'wika.governance.eval_schedule.enabled',
      'wika.governance.org_share.enabled',
    ],
  },
]

const modelServiceSections: Array<{
  key: ServiceSectionKey
  icon: string
  labelKey: string
  descriptionKey: string
}> = [
  {
    key: 'chat',
    icon: 'chat',
    labelKey: 'system.globalSettings.modelServices.chat.label',
    descriptionKey: 'system.globalSettings.modelServices.chat.description',
  },
  {
    key: 'embedding',
    icon: 'data-base',
    labelKey: 'system.globalSettings.modelServices.embedding.label',
    descriptionKey: 'system.globalSettings.modelServices.embedding.description',
  },
  {
    key: 'vllm',
    icon: 'image',
    labelKey: 'system.globalSettings.modelServices.vllm.label',
    descriptionKey: 'system.globalSettings.modelServices.vllm.description',
  },
  {
    key: 'asr',
    icon: 'sound',
    labelKey: 'system.globalSettings.modelServices.asr.label',
    descriptionKey: 'system.globalSettings.modelServices.asr.description',
  },
  {
    key: 'storage',
    icon: 'cloud',
    labelKey: 'system.globalSettings.modelServices.storage.label',
    descriptionKey: 'system.globalSettings.modelServices.storage.description',
  },
  {
    key: 'vectorstore',
    icon: 'data-base',
    labelKey: 'system.globalSettings.modelServices.vectorstore.label',
    descriptionKey: 'system.globalSettings.modelServices.vectorstore.description',
  },
  {
    key: 'parser',
    icon: 'file-search',
    labelKey: 'system.globalSettings.modelServices.parser.label',
    descriptionKey: 'system.globalSettings.modelServices.parser.description',
  },
]

const activeTab = ref<SystemTabKey>('platform-runtime')
const activeServiceSection = ref<ServiceSectionKey>('chat')

const visible = computed(() => uiStore.showSystemSettingsModal && authStore.isSystemAdmin)

const currentTab = computed(() => {
  return systemSettingTabs.find((tab) => tab.key === activeTab.value) ?? systemSettingTabs[0]
})

const settingsByKey = computed(() => {
  return new Map(settings.value.map((item) => [item.key, item]))
})

const settingGroupsForActiveTab = computed(() => {
  return SYSTEM_SETTING_GROUPS.filter((group) => group.tab === activeTab.value)
})

const activeModelSettingsType = computed<ModelSettingsInitialType>(() => {
  if (
    activeServiceSection.value === 'chat' ||
    activeServiceSection.value === 'embedding' ||
    activeServiceSection.value === 'vllm' ||
    activeServiceSection.value === 'asr'
  ) {
    return activeServiceSection.value
  }
  return 'all'
})

function setActiveTab(key: SystemTabKey) {
  activeTab.value = key
}

function setActiveServiceSection(key: ServiceSectionKey) {
  activeServiceSection.value = key
}

function groupItems(group: SettingGroupDefinition): SystemSettingItem[] {
  return group.settingKeys
    .map((key) => settingsByKey.value.get(key))
    .filter((item): item is SystemSettingItem => Boolean(item))
}

function handleClose() {
  uiStore.closeSystemSettings()
  auditDrawerVisible.value = false
}

function handleEscape(e: KeyboardEvent) {
  if (e.key === 'Escape' && visible.value) {
    handleClose()
  }
}

// Friendly labels per key live in i18n (system.globalSettings.keyLabels.*).
// Adding a new entry there must accompany every new key registered in
// service/system_setting.go on the backend; locales without an entry
// fall back to the raw key so a misconfigured deploy still renders.
function keyLabel(k: string): string {
  const path = `system.globalSettings.keyLabels.${k}`
  return te(path) ? (t(path) as string) : k
}

// Descriptions are registered in Chinese on the backend for operator docs;
// user-facing copy lives in i18n (system.globalSettings.keyDescriptions.*).
function settingDescription(item: { key: string; description?: string }): string {
  const path = `system.globalSettings.keyDescriptions.${item.key}`
  if (te(path)) return t(path) as string
  return item.description ?? ''
}

// Enum keys whose change triggers a whole-value inline popconfirm before
// PUT. ssrf.whitelist is not here — it uses per-tag confirm instead.
const HIGH_RISK_KEYS = new Set<string>([
  'auth.registration_mode',
])

function isHighRiskKey(key: string): boolean {
  return HIGH_RISK_KEYS.has(key)
}

type PopconfirmBtn = { content: string; theme?: 'primary' | 'danger' | 'warning' }

// TDesign popconfirm defaults to trigger:click on its inner Popup. Inputs
// wrapped for programmatic confirm must override that, otherwise focus /
// click on the field opens an empty bubble before the user commits a change.
const PROGRAMMATIC_POPCONFIRM_PROPS = { trigger: 'context-menu' as const }

// Shared inline t-popconfirm controller (anchored to the control row,
// same interaction model as Reset / bulk-apply). Replaces modal dialogs.
// State must be reactive (not nested refs) so template bindings unwrap.
function createInlinePopconfirm() {
  const state = reactive({
    visible: false,
    content: '',
    theme: 'warning' as 'default' | 'warning' | 'danger',
    confirmBtn: { content: '', theme: 'primary' } as PopconfirmBtn,
  })
  let resolver: ((ok: boolean) => void) | null = null
  let settled = false

  function ask(opts: {
    content: string
    theme?: 'default' | 'warning' | 'danger'
    confirmBtn: PopconfirmBtn
  }): Promise<boolean> {
    state.content = opts.content
    state.theme = opts.theme ?? 'warning'
    state.confirmBtn = opts.confirmBtn
    settled = false
    return new Promise((resolve) => {
      resolver = resolve
      state.visible = true
    })
  }

  function finish(ok: boolean) {
    if (settled) return
    settled = true
    state.visible = false
    const r = resolver
    resolver = null
    r?.(ok)
  }

  function onVisibleChange(v: boolean) {
    if (!v && resolver) finish(false)
  }

  return Object.assign(state, { ask, finish, onVisibleChange })
}

const ssrfPopconfirm = createInlinePopconfirm()
const adminPopconfirm = createInlinePopconfirm()
const highRiskPopconfirm = createInlinePopconfirm()

// Friendly labels for enum options live in i18n
// (system.globalSettings.enumLabels.<key>.<value>). Falls back to the
// raw enum value when no translation exists.
function enumLabel(itemKey: string, optionValue: string): string {
  const path = `system.globalSettings.enumLabels.${itemKey}.${optionValue}`
  return te(path) ? (t(path) as string) : optionValue
}

const emptyListPlaceholder = computed(() => t('system.globalSettings.tagInputPlaceholder'))

const settings = ref<SystemSettingItem[]>([])
const loading = ref(false)
const savingKey = ref<string | null>(null)

// Admin management state. We keep two parallel structures:
//   - adminEmails: the v-model bound to the t-tag-input (excludes
//     current user; that's the visible source of truth).
//   - adminEmailToId: email → user UUID, populated from the list
//     endpoint. Needed because revoke takes a UUID, not an email.
// Both reset on every reload to avoid stale entries persisting after
// a peer SystemAdmin makes a change. adminBusy disables the input and
// shows the row spinner only while promote/revoke API calls are in
// flight — not while the inline popconfirm is waiting for a click.
const adminEmails = ref<string[]>([])
const adminEmailToId = ref<Record<string, string>>({})
const adminBusy = ref(false)

// Guards ssrf.whitelist while an async confirm roundtrip is in flight.
const listConfirmBusyKey = ref<string | null>(null)

// Bumped when the SSRF tag-input is snapped back to the saved list so
// Vue remounts the control and clears TDesign's internal tag state.
const ssrfTagInputKeys = reactive<Record<string, number>>({})

// Briefly blocks model updates while the SSRF tag-input remount settles.
const ssrfSnapLocked = ref(false)

// Reactive map of in-progress edits, keyed by setting key. We don't
// mutate the canonical `settings` array directly so a failed save
// leaves the original value visible until the user retries or refreshes.
// Initialised lazily in loadSettings; setting.value is the JSON-decoded
// form (number / boolean / string / string[]).
const editValues = reactive<Record<string, unknown>>({})

function hasEnum(item: SystemSettingItem): boolean {
  return Array.isArray(item.enum) && item.enum.length > 0
}

function enumOptions(item: SystemSettingItem): { label: string; value: string }[] {
  const opts = item.enum ?? []
  return opts.map((v) => ({ label: enumLabel(item.key, v), value: v }))
}

type ModelSettingType = 'KnowledgeQA' | 'Embedding' | 'VLLM' | 'ASR'

const MODEL_SETTING_TYPES: Record<string, ModelSettingType> = {
  'kb.default_llm_model_id': 'KnowledgeQA',
  'kb.default_embedding_model_id': 'Embedding',
  'kb.default_vlm_model_id': 'VLLM',
  'kb.default_asr_model_id': 'ASR',
}

const MODEL_SETTING_SUB_SECTIONS: Record<ModelSettingType, ServiceSectionKey> = {
  KnowledgeQA: 'chat',
  Embedding: 'embedding',
  VLLM: 'vllm',
  ASR: 'asr',
}

const RELATED_SETTING_ACTIONS: Record<string, { labelKey: string; icon: string; serviceSection: ServiceSectionKey }> = {
  'kb.default_storage_provider': {
    labelKey: 'system.globalSettings.relatedActions.manageStorage',
    icon: 'cloud',
    serviceSection: 'storage',
  },
  'kb.default_index_vector_enabled': {
    labelKey: 'system.globalSettings.relatedActions.manageVectorStore',
    icon: 'data-base',
    serviceSection: 'vectorstore',
  },
}

function isModelSetting(key: string): boolean {
  return Object.prototype.hasOwnProperty.call(MODEL_SETTING_TYPES, key)
}

function modelSettingType(key: string): ModelSettingType {
  return MODEL_SETTING_TYPES[key] ?? 'KnowledgeQA'
}

function openModelSettings(key: string) {
  activeServiceSection.value = MODEL_SETTING_SUB_SECTIONS[modelSettingType(key)]
  activeTab.value = 'models-services'
  scrollServiceSection(activeServiceSection.value)
}

function relatedActionFor(item: SystemSettingItem): { labelKey: string; icon: string; serviceSection: ServiceSectionKey } | null {
  if (isModelSetting(item.key)) {
    return {
      labelKey: 'system.globalSettings.relatedActions.manageModel',
      icon: 'control-platform',
      serviceSection: MODEL_SETTING_SUB_SECTIONS[modelSettingType(item.key)],
    }
  }
  return RELATED_SETTING_ACTIONS[item.key] ?? null
}

function hasRelatedAction(item: SystemSettingItem): boolean {
  return relatedActionFor(item) !== null
}

function relatedActionIcon(item: SystemSettingItem): string {
  return relatedActionFor(item)?.icon ?? 'link'
}

function relatedActionLabel(item: SystemSettingItem): string {
  const action = relatedActionFor(item)
  return action ? t(action.labelKey) : ''
}

function openRelatedArea(item: SystemSettingItem) {
  const action = relatedActionFor(item)
  if (!action) return
  activeServiceSection.value = action.serviceSection
  activeTab.value = 'models-services'
  scrollServiceSection(action.serviceSection)
}

function scrollServiceSection(section: ServiceSectionKey) {
  nextTick(() => {
    const el = document.querySelector(`[data-service-section="${section}"]`)
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }
  })
}

// hasOverride reports whether the row carries a real DB override (vs a
// virtual row backed by ENV/default). Distinguishing these is what
// `last_modified_by` was made for: empty string means the value came
// from registry/ENV. Drives the "已覆盖" badge.
function hasOverride(item: SystemSettingItem): boolean {
  return Boolean(item.last_modified_by && item.last_modified_by.trim() !== '')
}

// modifiedMeta returns a humane "上次修改" line for rows that have been
// persisted (last_modified_by non-empty AND updated_at not the Go zero
// value). Returns '' for virtual rows so the meta line collapses
// entirely instead of rendering "1/1/1 08:05:43" garbage.
function modifiedMeta(item: SystemSettingItem): string {
  if (!hasOverride(item)) return ''
  const ts = item.updated_at
  if (!ts || ts.startsWith('0001-')) return ''
  const formatted = formatDate(ts)
  // Prefer the resolved username/email the server enriches via
  // last_modified_by_name. Fall back to the UUID's first 8 chars when
  // the user can't be resolved (deleted account, transient lookup
  // failure) — the full ID is still in the audit log.
  const actor = item.last_modified_by_name && item.last_modified_by_name.trim() !== ''
    ? item.last_modified_by_name
    : (item.last_modified_by || '').slice(0, 8)
  return `${formatted} · ${actor}`
}

const SSRF_WHITELIST_KEY = 'ssrf.whitelist'

function ssrfWhitelistModelValue(): string[] {
  const v = editValues[SSRF_WHITELIST_KEY]
  return Array.isArray(v) ? (v as string[]) : []
}

function ssrfTagInputKey(): number {
  return ssrfTagInputKeys[SSRF_WHITELIST_KEY] ?? 0
}

function resetSsrfTagInput() {
  ssrfTagInputKeys[SSRF_WHITELIST_KEY] = (ssrfTagInputKeys[SSRF_WHITELIST_KEY] ?? 0) + 1
}

function globalSettingsText(path: string, params?: Record<string, string>): string {
  if (!te(path)) return path
  const msg = params ? t(path, params) : t(path)
  return typeof msg === 'string' ? msg : path
}

// Controlled SSRF tag-input: we commit editValues so a declined delta
// can be rolled back without the component re-applying a removal.
function onSsrfWhitelistModelUpdate(next: string[]) {
  if (listConfirmBusyKey.value === SSRF_WHITELIST_KEY || ssrfSnapLocked.value) return
  editValues[SSRF_WHITELIST_KEY] = next
  void onSsrfWhitelistChange()
}

async function onSsrfWhitelistChange() {
  const item = settings.value.find((s) => s.key === SSRF_WHITELIST_KEY)
  if (!item || !isDirty(item)) return
  if (listConfirmBusyKey.value === SSRF_WHITELIST_KEY) return
  await handleSSRFListChange(item)
}

async function snapSsrfWhitelistToSaved(item: SystemSettingItem) {
  const saved = Array.isArray(item.value) ? (item.value as string[]) : []
  editValues[SSRF_WHITELIST_KEY] = [...saved]
  resetSsrfTagInput()
  ssrfSnapLocked.value = true
  await nextTick()
  await nextTick()
  ssrfSnapLocked.value = false
}

function isDirty(item: SystemSettingItem): boolean {
  const cur = editValues[item.key]
  const orig = item.value
  if (Array.isArray(cur) && Array.isArray(orig)) {
    if (cur.length !== orig.length) return true
    for (let i = 0; i < cur.length; i++) {
      if (cur[i] !== orig[i]) return true
    }
    return false
  }
  return cur !== orig
}

function formatDate(isoString: string): string {
  try {
    const d = new Date(isoString)
    return d.toLocaleString('zh-CN', { hour12: false })
  } catch {
    return isoString
  }
}

// placeholderFor renders the current effective value (DB / ENV / default)
// as a placeholder hint inside the edit control. For string_list it's
// joined with comma; for booleans we show nothing (the switch already
// reflects the value).
function placeholderFor(item: SystemSettingItem): string {
  const v = item.value
  if (v === null || v === undefined) return ''
  if (Array.isArray(v)) return v.join(', ')
  return String(v)
}

async function loadSettings() {
  loading.value = true
  try {
    const list = await listSystemSettings()
    settings.value = list
    // Reset edit values to the canonical state on every load — no
    // partial drafts survive a refresh, which avoids the "I came back
    // and my unsaved edits look saved" trap.
    for (const item of list) {
      // Defensive copy for arrays so the t-tag-input doesn't mutate
      // the canonical settings entry through the v-model binding.
      editValues[item.key] = Array.isArray(item.value)
        ? [...(item.value as unknown[])]
        : item.value
    }
  } catch (err: any) {
    const msg = err?.message || t('system.globalSettings.messages.loadFailed')
    MessagePlugin.error(msg)
  } finally {
    loading.value = false
  }
}

// onChange persists non-SSRF settings. SSRF whitelist and system admins
// have dedicated handlers with inline popconfirm.
async function onChange(item: SystemSettingItem) {
  if (!isDirty(item)) return

  // SSRF whitelist gets the per-entry confirm flow — same shape as the
  // admin tag-input above. Adding or removing each host/CIDR is its
  // own privileged change (a single bad CIDR can punch a hole through
  // the egress firewall), so we ask once per delta instead of once
  // per "save". This matches the operator's mental model: every tag
  // they touch is acknowledged on its own.
  await persistSetting(item)
}

async function onModelSettingChange(item: SystemSettingItem, value: string) {
  editValues[item.key] = value
  await onChange(item)
}

async function onHighRiskSelectChange(item: SystemSettingItem) {
  const newValue = editValues[item.key]
  if (newValue === item.value) return

  // Revert the select immediately so cancel leaves the saved value
  // visible; re-apply only after the inline popconfirm is confirmed.
  editValues[item.key] = item.value

  const ok = await highRiskPopconfirm.ask({
    content: highRiskConfirmBody(item, newValue),
    theme: 'danger',
    confirmBtn: {
      content: t('system.globalSettings.confirm.confirmBtn'),
      theme: 'danger',
    },
  })
  if (!ok) return

  editValues[item.key] = newValue
  await persistSetting(item)
}

function confirmSsrfListEntryChange(
  action: 'add' | 'remove',
  entry: string,
): Promise<boolean> {
  const base = `system.globalSettings.listConfirm.${SSRF_WHITELIST_KEY}.${action}`
  return ssrfPopconfirm.ask({
    content: globalSettingsText(`${base}.body`, { entry }),
    theme: action === 'add' ? 'danger' : 'warning',
    confirmBtn: {
      content: globalSettingsText(`${base}.confirmBtn`),
      theme: action === 'add' ? 'danger' : 'primary',
    },
  })
}

// handleSSRFListChange reconciles the current edit against the saved
// list one entry at a time. The strategy is "confirmed deltas only":
// we start from the saved value, then walk the user's added/removed
// sets and apply each entry the operator individually approves. If
// every prompt is declined we end up identical to the saved value
// and short-circuit before hitting the API. Otherwise we save the
// merged result in a single PUT so the audit log and pubsub get one
// coherent post-image (instead of N noisy events).
async function handleSSRFListChange(item: SystemSettingItem) {
  listConfirmBusyKey.value = item.key
  try {
    const oldArr = Array.isArray(item.value) ? (item.value as string[]) : []
    const nextArr = Array.isArray(editValues[item.key])
      ? (editValues[item.key] as string[])
      : []

    const oldSet = new Set(oldArr)
    const nextSet = new Set(nextArr)

    const added: string[] = []
    for (const v of nextSet) if (!oldSet.has(v)) added.push(v)
    const removed: string[] = []
    for (const v of oldSet) if (!nextSet.has(v)) removed.push(v)

    if (added.length === 0 && removed.length === 0) return

    // Build the candidate value from approved deltas only. We keep
    // insertion order roughly aligned with the operator's intent:
    // start from the saved list (so unchanged entries keep their
    // position), drop approved removals, append approved additions.
    const finalSet = new Set(oldArr)
    for (const entry of added) {
      const ok = await confirmSsrfListEntryChange('add', entry)
      if (ok) {
        finalSet.add(entry)
      } else {
        await snapSsrfWhitelistToSaved(item)
        return
      }
    }
    for (const entry of removed) {
      const ok = await confirmSsrfListEntryChange('remove', entry)
      if (ok) {
        finalSet.delete(entry)
      } else {
        await snapSsrfWhitelistToSaved(item)
        return
      }
    }

    const finalArr = Array.from(finalSet)
    // Compare against saved value, not against `editValues`. If every
    // delta was declined, the saved list still wins; we just need to
    // snap the input back to it.
    const sameAsSaved =
      finalArr.length === oldArr.length &&
      finalArr.every((v, i) => v === oldArr[i])
    if (sameAsSaved) {
      await snapSsrfWhitelistToSaved(item)
      return
    }

    editValues[item.key] = finalArr
    await persistSetting(item)
  } finally {
    await nextTick()
    listConfirmBusyKey.value = null
  }
}

function highRiskConfirmBody(item: SystemSettingItem, value: unknown): string {
  const renderedValue = Array.isArray(value)
    ? value.length === 0
      ? t('system.globalSettings.confirm.emptyValue')
      : value.join(', ')
    : String(value)
  return t('system.globalSettings.confirm.bodyAuthRegistrationMode', {
    label: keyLabel(item.key),
    value: renderedValue,
  })
}

// hasBulkAction tells the template whether the current row carries an
// extra "apply to existing data" action beyond plain save/reset.
// Currently only `tenant.default_storage_quota_gb` does — saving the
// setting only affects future tenants, so the bulk button is the
// escape hatch for "rewrite all current tenants too".
function hasBulkAction(item: SystemSettingItem): boolean {
  return item.key === 'tenant.default_storage_quota_gb'
}

function bulkActionConfirmBody(item: SystemSettingItem): string {
  // Use the canonical (saved) value, not the in-progress edit, so the
  // operator sees exactly what will be written. The button is disabled
  // when the row is dirty (see template), so item.value is the value
  // that's currently in effect for new tenants.
  const v = item.value
  const valueText = v === null || v === undefined ? '' : String(v)
  return t('system.globalSettings.bulkApply.confirmBody', { value: valueText })
}

async function runBulkAction(item: SystemSettingItem) {
  if (!hasBulkAction(item)) return
  savingKey.value = item.key
  try {
    const result = await applyDefaultStorageQuotaToAllTenants()
    MessagePlugin.success(
      t('system.globalSettings.bulkApply.success', {
        count: result.affected,
        gb: result.quota_gb,
      }),
    )
  } catch (err: any) {
    const msg = err?.message || t('system.globalSettings.bulkApply.failed')
    MessagePlugin.error(msg)
  } finally {
    savingKey.value = null
  }
}

// resetSetting drops the DB override and reloads the row so the UI
// reflects the resolved fallback (ENV value if set, otherwise the
// in-code default). We refetch the whole list rather than the single
// row because the list endpoint is what populates the canonical
// settings array and re-running it keeps the modified-by enrichment
// consistent for every row in the table.
async function resetSetting(item: SystemSettingItem) {
  savingKey.value = item.key
  try {
    await resetSystemSetting(item.key)
    await loadSettings()
    MessagePlugin.success(t('system.globalSettings.reset.success'))
  } catch (err: any) {
    const msg = err?.message || t('system.globalSettings.reset.failed')
    MessagePlugin.error(msg)
  } finally {
    savingKey.value = null
  }
}

async function persistSetting(item: SystemSettingItem) {
  const newValue = editValues[item.key]
  savingKey.value = item.key
  try {
    const updated = await updateSystemSetting(item.key, newValue)
    // Replace the row in-place so the table stays at scroll position
    // and other rows' edit state isn't disturbed.
    const idx = settings.value.findIndex((s) => s.key === item.key)
    if (idx >= 0) {
      settings.value[idx] = updated
    }
    editValues[item.key] = Array.isArray(updated.value)
      ? [...(updated.value as unknown[])]
      : updated.value
    MessagePlugin.success(t('system.globalSettings.messages.saveSuccess'))
  } catch (err: any) {
    const msg = err?.message || t('system.globalSettings.messages.saveFailed')
    MessagePlugin.error(msg)
    // Roll the input back to the canonical value on failure. Without
    // this an invalid edit (e.g. SSRF whitelist with a malformed CIDR
    // that the backend 400'd) would stay rendered as if accepted, and
    // the user couldn't tell whether the rejection actually stuck.
    const failed = settings.value.find((s) => s.key === item.key)
    if (failed) {
      editValues[item.key] = Array.isArray(failed.value)
        ? [...(failed.value as unknown[])]
        : failed.value
    }
  } finally {
    savingKey.value = null
  }
}

// loadAdmins refreshes the admin tag list + the email→id lookup
// table. We exclude the current user from the visible list so the
// "you can't revoke yourself" rule has nothing to enforce in the UI
// (the backend rejects it too, but hiding the tag is friendlier).
async function loadAdmins() {
  try {
    const resp = await listSystemAdmins({ limit: 200 })
    const map: Record<string, string> = {}
    const emails: string[] = []
    for (const u of resp.admins ?? []) {
      // Empty emails would collapse to a single tag "" that can't be
      // round-tripped to a user_id; skip them. Same defensive stance
      // as resolveMaxOwnedTenantsPerUser on the backend.
      if (!u.email) continue
      map[u.email] = u.id
      if (u.id !== currentUserId.value) {
        emails.push(u.email)
      }
    }
    adminEmailToId.value = map
    adminEmails.value = emails
  } catch (err: any) {
    const msg = err?.message || t('system.globalSettings.admins.loadFailed')
    MessagePlugin.error(msg)
  }
}

function confirmAdminChange(action: 'promote' | 'revoke', email: string): Promise<boolean> {
  const base = `system.globalSettings.admins.confirm.${action}`
  return adminPopconfirm.ask({
    content: globalSettingsText(`${base}.body`, { email }),
    theme: action === 'revoke' ? 'danger' : 'warning',
    confirmBtn: {
      content: globalSettingsText(`${base}.confirmBtn`),
      theme: action === 'revoke' ? 'danger' : 'primary',
    },
  })
}

// onAdminsChange diffs the new tag list against the canonical state
// and dispatches one promote / revoke per delta. Failures roll back
// the whole tag list to the server-side truth — this is simpler than
// trying to undo individual ops, and the network/error case for batch
// edits is rare enough that a full reload doesn't surprise anyone.
async function onAdminsChange(next: string[]) {
  if (adminBusy.value) return

  // Snapshot of what's currently authoritative — the email→id map's
  // keys, minus the current user. Anything in `next` that's not here
  // is an addition; anything here that's not in `next` is a removal.
  const authoritative = new Set<string>()
  for (const email of Object.keys(adminEmailToId.value)) {
    if (adminEmailToId.value[email] !== currentUserId.value) {
      authoritative.add(email)
    }
  }
  const nextSet = new Set(next.map((e) => e.trim()).filter(Boolean))

  // Drop the user-typed entry to canonical lowercase/trim before we
  // diff. We don't lowercase server-returned emails because the
  // backend stores the original case; matching against the map's keys
  // happens with the as-typed value, which is what the user sees.
  const added: string[] = []
  for (const email of nextSet) {
    if (!authoritative.has(email)) added.push(email)
  }
  const removed: string[] = []
  for (const email of authoritative) {
    if (!nextSet.has(email)) removed.push(email)
  }

  if (added.length === 0 && removed.length === 0) return

  // Confirm before any privilege change (no loading spinner yet — the
  // popconfirm is the only UI; adminBusy is reserved for API roundtrips).
  for (const email of added) {
    const ok = await confirmAdminChange('promote', email)
    if (!ok) {
      await loadAdmins()
      return
    }
  }
  for (const email of removed) {
    const userId = adminEmailToId.value[email]
    if (!userId) continue
    const ok = await confirmAdminChange('revoke', email)
    if (!ok) {
      await loadAdmins()
      return
    }
  }

  adminBusy.value = true
  let applied = 0
  try {
    for (const email of added) {
      await promoteUserToSystemAdmin({ email })
      applied++
    }
    for (const email of removed) {
      const userId = adminEmailToId.value[email]
      if (!userId) continue
      await revokeSystemAdmin(userId)
      applied++
    }
    await loadAdmins()
    if (applied > 0) {
      MessagePlugin.success(t('system.globalSettings.admins.saveSuccess'))
    }
  } catch (err: any) {
    const msg = err?.message || t('system.globalSettings.admins.saveFailed')
    MessagePlugin.error(msg)
    await loadAdmins()
  } finally {
    adminBusy.value = false
  }
}

watch(
  () => uiStore.showSystemSettingsModal,
  async (open) => {
    if (!open) return
    if (!authStore.isSystemAdmin) {
      uiStore.closeSystemSettings()
      return
    }
    const initialSection = uiStore.systemSettingsInitialSection
    if (initialSection && systemSettingTabs.some((tab) => tab.key === initialSection)) {
      activeTab.value = initialSection as SystemTabKey
    }
    const initialSubSection = uiStore.systemSettingsInitialSubSection
    if (initialSubSection && modelServiceSections.some((section) => section.key === initialSubSection)) {
      activeServiceSection.value = initialSubSection as ServiceSectionKey
    }
    await Promise.all([loadSettings(), loadAdmins()])
  },
)

watch(
  () => authStore.isSystemAdmin,
  (isSystemAdmin) => {
    if (!isSystemAdmin && uiStore.showSystemSettingsModal) {
      uiStore.closeSystemSettings()
    }
  },
)

onMounted(() => {
  window.addEventListener('keydown', handleEscape)
})

// ---- Platform audit log (system-scope, tenant_id=0) ---------------------
//
// Wired against GET /api/v1/system/admin/audit-log (SystemAdmin only).
// The drawer mirrors the structural choices of the tenant audit drawer
// in frontend/src/views/settings/TenantMembers.vue: cursor-paged by
// descending id, lazy-loaded on first open, infinite-scroll via an
// IntersectionObserver pinned to the scroll root. Refresh is explicit
// via a button inside the drawer so closing/reopening doesn't quietly
// fire a new fetch the operator didn't ask for.

const auditDrawerVisible = ref(false)
const auditEntries = ref<AuditLog[]>([])
const auditLoading = ref(false)
const auditError = ref('')
const auditCursor = ref<number>(0) // 0 = "from the top"
const auditHasMore = ref(true)
const auditLoadedOnce = ref(false)
const AUDIT_PAGE_SIZE = 50

const auditScrollRoot = ref<HTMLElement | null>(null)
const auditLoadSentinelEl = ref<HTMLElement | null>(null)
let auditScrollObserver: IntersectionObserver | null = null

// We render a stacked "date / time" cell rather than ellipsing a single
// flat string — the screenshot review surfaced that the joined form
// reads as a wall of identical timestamps when 50 events fall in the
// same minute. A two-line cell also frees horizontal space for the
// (much more important) target diff column.

const auditColumns = computed(() => [
  { colKey: 'created_at', title: t('system.globalSettings.audit.columns.time'), width: 120 },
  { colKey: 'actor', title: t('system.globalSettings.audit.columns.actor'), width: 180 },
  { colKey: 'action', title: t('system.globalSettings.audit.columns.action'), width: 150 },
  {
    colKey: 'target',
    title: t('system.globalSettings.audit.columns.target'),
    // No fixed width / no ellipsis: this is where the diff content
    // lives, and clipping it to "..." negates the entire reason we
    // synthesise the cell in the first place. CSS handles wrapping.
    minWidth: 240,
  },
  { colKey: 'outcome', title: t('system.globalSettings.audit.columns.outcome'), width: 80, align: 'center' as const },
])

// Two helpers feeding the stacked time cell. Falling back to the raw
// string keeps the table readable when Intl chokes on a malformed
// timestamp (shouldn't happen, but cheap to defend).
function formatAuditDatePart(s: string | undefined): string {
  if (!s) return '-'
  try {
    return new Intl.DateTimeFormat(locale.value || 'zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    }).format(new Date(s))
  } catch {
    return s
  }
}

function formatAuditTimePart(s: string | undefined): string {
  if (!s) return ''
  try {
    return new Intl.DateTimeFormat(locale.value || 'zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    }).format(new Date(s))
  } catch {
    return ''
  }
}

// Action chip colour: promote is reassuring green; revoke / setting
// change are worth a second look (warning orange); denied / access
// rejections show danger so an operator can scan a chronological feed
// and immediately spot abuse.
function auditActionTheme(
  action: AuditAction,
): 'success' | 'warning' | 'danger' | 'primary' | 'default' {
  switch (action) {
    case 'system.admin_promoted':
      return 'success'
    case 'system.admin_revoked':
    case 'system.setting_changed':
      return 'warning'
    case 'rbac.access_denied':
      return 'danger'
    default:
      return 'default'
  }
}

function auditOutcomeTheme(o: AuditOutcome): 'success' | 'danger' | 'default' {
  if (o === 'denied') return 'danger'
  if (o === 'success') return 'success'
  return 'default'
}

// i18n 键名含点号（system.setting_changed）。用 t(path) 会按路径拆开解析，
// 无法命中 system.globalSettings.audit.action['system.*'] — 必须 tm + 字面量键。
function formatAuditAction(action: AuditAction): string {
  const bag = tm('system.globalSettings.audit.action') as unknown
  if (bag !== null && typeof bag === 'object' && typeof (bag as Record<string, string>)[action] === 'string') {
    return (bag as Record<string, string>)[action]
  }
  return action
}

// Actor display: most system-admin operations are performed by humans
// whose username we don't have a local mirror of. The audit row only
// carries the UUID, so we fall back to a short prefix for readability.
// If the actor is the current user, we resolve to their own profile.
function auditActorLabel(userId: string): string {
  const me = authStore.user
  if (me && me.id === userId) {
    return me.username?.trim() || me.email?.trim() || userId.slice(0, 8)
  }
  return userId.slice(0, 8)
}

function auditActorRoleLabel(role: string): string {
  const key = `system.globalSettings.audit.actorRole.${role}`
  if (te(key)) return t(key)
  return role
}

// Target rendering is split into two pieces so the table cell can
// show a structural "subject" (key / user) on its own line and the
// value diff on a second, monospaced line — far more legible than a
// single concatenated string clipped by ellipsis.

function auditDetailsObject(row: AuditLog): Record<string, unknown> | null {
  if (row.details && typeof row.details === 'object') {
    return row.details as Record<string, unknown>
  }
  return null
}

// First line of the target cell — the thing being acted on.
//   - setting_changed (regular key): the registry key
//   - setting_changed (bulk apply):  i18n label "(bulk) default storage quota"
//   - admin_promoted/revoked:        username (email) of the affected user
function auditTargetKey(row: AuditLog): string {
  const details = auditDetailsObject(row)
  if (row.action === 'system.setting_changed') {
    if (row.target_type === 'tenant_storage_quota') {
      return t('system.globalSettings.audit.target.bulkQuota')
    }
    if (details && typeof details.key === 'string' && details.key) return details.key
    return row.target_id || row.target_type || ''
  }
  if (row.action === 'system.admin_promoted' || row.action === 'system.admin_revoked') {
    if (!details) return row.target_user_id ? row.target_user_id.slice(0, 8) : ''
    const name = typeof details.target_username === 'string' ? details.target_username : ''
    const mail = typeof details.target_email === 'string' ? details.target_email : ''
    if (name && mail) return `${name} (${mail})`
    return name || mail || (row.target_user_id ? row.target_user_id.slice(0, 8) : '')
  }
  if (row.target_user_id) return row.target_user_id.slice(0, 8)
  if (row.target_id) {
    return row.target_type ? `${row.target_type}:${row.target_id}` : row.target_id
  }
  return ''
}

// Second line — the change diff. Returns an empty string when there
// is no meaningful diff to display (the expanded row still surfaces
// the raw JSON for forensics).
function auditTargetDiff(row: AuditLog): string {
  const details = auditDetailsObject(row)
  if (!details) return ''
  if (row.action === 'system.setting_changed') {
    if (row.target_type === 'tenant_storage_quota') {
      const affected = typeof details.affected === 'number' ? details.affected : null
      const gb = typeof details.quota_gb === 'number' ? details.quota_gb : null
      if (affected !== null && gb !== null) {
        return t('system.globalSettings.audit.target.bulkQuotaDiff', {
          count: String(affected),
          gb: String(gb),
        })
      }
      return ''
    }
    return formatSettingDiff(details)
  }
  if (row.action === 'system.admin_promoted' && typeof details.idempotent === 'boolean') {
    if (details.idempotent === true) {
      return t('system.globalSettings.audit.target.promoteIdempotent')
    }
    return ''
  }
  if (row.action === 'system.admin_revoked' && typeof details.changed === 'boolean') {
    if (details.changed === false) {
      return t('system.globalSettings.audit.target.revokeNoop')
    }
    return ''
  }
  if (row.action === 'rbac.access_denied' && typeof details.required_role === 'string') {
    return t('system.globalSettings.audit.target.requiredRole', { role: details.required_role })
  }
  return ''
}

const SETTING_DIFF_MAX_LEN = 80
function formatSettingDiff(details: Record<string, unknown>): string {
  const fmt = (v: unknown): string => {
    if (v === null || v === undefined) {
      return t('system.globalSettings.audit.target.valueNull')
    }
    if (typeof v === 'string') return v
    if (typeof v === 'number' || typeof v === 'boolean') return String(v)
    try {
      return JSON.stringify(v)
    } catch {
      return String(v)
    }
  }
  const truncate = (s: string): string =>
    s.length > SETTING_DIFF_MAX_LEN ? s.slice(0, SETTING_DIFF_MAX_LEN - 1) + '…' : s
  const oldStr = truncate(fmt(details.old_value))
  const newStr = truncate(fmt(details.new_value))
  if (oldStr === newStr) return ''
  return `${oldStr} → ${newStr}`
}

// Expanded row state — local set of row ids the user has opened.
// We keep it ephemeral (not persisted) so reopening the drawer always
// shows a clean, collapsed view.
const auditExpandedRowKeys = ref<number[]>([])

function onAuditExpandChange(value: (string | number)[]) {
  // t-table calls back with the *new* full list of expanded keys.
  // Normalise to numbers because AuditLog.id is always a number.
  auditExpandedRowKeys.value = value
    .map((v) => (typeof v === 'number' ? v : Number(v)))
    .filter((v) => Number.isFinite(v))
}

function auditDetailsJSON(row: AuditLog): string {
  if (row.details === null || row.details === undefined) return '{}'
  if (typeof row.details === 'string') return row.details
  try {
    return JSON.stringify(row.details, null, 2)
  } catch {
    return String(row.details)
  }
}

async function loadAuditLog(reset: boolean) {
  if (auditLoading.value) return
  if (!reset && !auditHasMore.value) return

  auditLoading.value = true
  auditError.value = ''
  try {
    const resp = await listSystemAuditLog({
      after_id: reset ? undefined : auditCursor.value || undefined,
      limit: AUDIT_PAGE_SIZE,
    })
    if (resp.success) {
      const rows = resp.data || []
      auditEntries.value = reset ? rows : [...auditEntries.value, ...rows]
      auditCursor.value = resp.next_cursor || 0
      // Same convention as tenant audit: next_cursor=0 means "no
      // older rows", regardless of whether the current page was empty.
      auditHasMore.value = !!resp.next_cursor && rows.length > 0
      auditLoadedOnce.value = true
    } else {
      auditError.value = resp.message || t('system.globalSettings.audit.errors.generic')
    }
  } catch (err: any) {
    const status = err?.status
    if (status === 403) {
      auditError.value = t('system.globalSettings.audit.forbidden')
    } else {
      auditError.value = err?.message || t('system.globalSettings.audit.errors.generic')
    }
  } finally {
    auditLoading.value = false
  }
}

function detachAuditInfiniteScroll() {
  auditScrollObserver?.disconnect()
  auditScrollObserver = null
}

function attachAuditInfiniteScroll() {
  detachAuditInfiniteScroll()
  const root = auditScrollRoot.value
  const sentinel = auditLoadSentinelEl.value
  if (!root || !sentinel) return

  auditScrollObserver = new IntersectionObserver(
    (entries) => {
      const hitBottom = entries.some((e) => e.isIntersecting)
      if (!hitBottom || !auditHasMore.value || auditLoading.value) return
      void loadAuditLog(false)
    },
    { root, rootMargin: '100px 0px', threshold: 0 },
  )
  auditScrollObserver.observe(sentinel)
}

function reloadAuditLog() {
  auditCursor.value = 0
  auditHasMore.value = true
  loadAuditLog(true)
}

function openAuditDrawer() {
  auditDrawerVisible.value = true
  if (!auditLoadedOnce.value) {
    loadAuditLog(true)
  }
}

watch(
  auditDrawerVisible,
  async (open) => {
    if (!open) {
      detachAuditInfiniteScroll()
      return
    }
    await nextTick()
    attachAuditInfiniteScroll()
  },
  { flush: 'post' },
)

watch(
  () => auditError.value,
  async () => {
    if (!auditDrawerVisible.value) return
    await nextTick()
    if (!auditError.value) {
      attachAuditInfiniteScroll()
      return
    }
    detachAuditInfiniteScroll()
  },
  { flush: 'post' },
)

onUnmounted(() => {
  window.removeEventListener('keydown', handleEscape)
  detachAuditInfiniteScroll()
})
</script>

<style lang="less" scoped>
.system-settings {
  width: 100%;
}

.system-settings-overlay {
  position: fixed;
  inset: 0;
  z-index: 1120;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
}

.system-settings-modal {
  position: relative;
  width: 100%;
  max-width: 1120px;
  height: 820px;
  max-height: calc(100vh - 40px);
  background: var(--td-bg-color-container);
  border-radius: 12px;
  box-shadow: 0 6px 28px rgba(15, 23, 42, 0.08);
  overflow: hidden;
}

.system-settings-close {
  position: absolute;
  top: 16px;
  right: 16px;
  z-index: 10;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;

  &:hover {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }
}

.system-settings-container {
  display: flex;
  width: 100%;
  height: 100%;
  min-height: 0;
}

.system-settings-sidebar {
  width: 224px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--td-bg-color-settings-modal);
  border-right: 1px solid var(--td-component-stroke);
  overflow: hidden;
}

.system-settings-sidebar-header {
  padding: 18px 16px 14px;
  border-bottom: 1px solid var(--td-component-stroke);

  h2 {
    margin: 0 0 8px;
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  p {
    margin: 0;
    font-size: 12px;
    line-height: 1.5;
    color: var(--td-text-color-secondary);
  }
}

.system-settings-nav {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 10px 8px 12px;
}

.system-settings-nav-item {
  width: 100%;
  height: 34px;
  border: none;
  border-radius: 6px;
  margin-bottom: 4px;
  padding: 0 12px;
  background: transparent;
  color: var(--td-text-color-primary);
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 9px;
  font-size: 14px;
  text-align: left;

  &:hover {
    background: var(--td-bg-color-container-hover);
  }

  &.active {
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-brand-color);
    font-weight: 500;
  }
}

.system-settings-nav-icon {
  font-size: 16px;
  flex-shrink: 0;
}

.system-settings-content {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.system-settings-content-header {
  min-height: 88px;
  flex-shrink: 0;
  padding: 22px 56px 16px 28px;
  border-bottom: 1px solid var(--td-component-stroke);
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;

  h2 {
    margin: 0 0 8px;
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  p {
    margin: 0;
    font-size: 14px;
    line-height: 1.5;
    color: var(--td-text-color-secondary);
  }
}

.system-settings-content-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 24px 28px 32px;
}

.system-settings-group {
  margin-bottom: 26px;
}

.setting-group-header {
  margin-bottom: 4px;

  h3 {
    margin: 0 0 6px;
    font-size: 15px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  p {
    margin: 0;
    font-size: 13px;
    line-height: 1.5;
    color: var(--td-text-color-secondary);
  }
}

.model-service-intro {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
  gap: 10px;
  margin-bottom: 20px;
}

.model-service-item {
  min-height: 78px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  padding: 12px;
  cursor: pointer;
  display: flex;
  gap: 10px;
  background: var(--td-bg-color-container);

  &:hover,
  &.active {
    border-color: var(--td-brand-color);
    background: var(--td-bg-color-secondarycontainer);
  }

  p {
    margin: 4px 0 0;
    font-size: 12px;
    line-height: 1.45;
    color: var(--td-text-color-secondary);
  }
}

.model-service-icon {
  flex-shrink: 0;
  margin-top: 2px;
  color: var(--td-brand-color);
}

.model-service-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.embedded-model-settings {
  &:deep(.section-header) {
    margin-top: 0;
  }
}

.section-header {
  margin-bottom: 24px;

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 8px 0;
  }

  .section-description {
    font-size: 14px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }
}

/* Title + audit-log entry sit on the same row, parallel to the layout
   used in tenant member settings — keeps secondary actions anchored to
   the section header instead of floating loose above content. */
.section-header-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;

  h2 {
    margin: 0;
  }
}

.header-audit-btn {
  flex-shrink: 0;
}

/* ===== Audit drawer (mirrors TenantMembers.vue's audit panel) =========
   Kept scoped to this view rather than extracted to a shared component:
   the two pages render distinct action labels and target formatters,
   and a generic <AuditLogPanel> would have to thread enough props
   through to make the abstraction more expensive than the duplication.
   Revisit if a third audit surface appears. */
.audit-panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding-top: 8px;
}

.audit-panel--drawer {
  padding-top: 0;
}

.audit-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--td-bg-color-secondarycontainer);
  padding: 12px 16px;
  border-radius: 8px;
  gap: 12px;

  .audit-desc {
    flex: 1;
    min-width: 0;
    font-size: 13px;
    color: var(--td-text-color-secondary);
  }

  .audit-refresh-btn {
    flex-shrink: 0;
  }
}

.audit-drawer-inner {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  gap: 14px;
  min-height: 0;
  width: 100%;
  box-sizing: border-box;
}

.audit-drawer-fill {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.audit-drawer-branch {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.audit-drawer-branch--error {
  justify-content: center;

  .error-inline {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 20px 0 8px;
  }
}

.audit-drawer-branch--empty.empty-state--audit {
  flex: 1 1 auto;
  justify-content: center;
  align-items: center;
  padding: 24px 12px;
  min-height: 0;
}

.audit-scroll-area {
  flex: 1 1 auto;
  min-height: 0;
  overflow-x: hidden;
  overflow-y: auto;
}

.audit-load-sentinel {
  height: 1px;
  width: 100%;
  pointer-events: none;
}

.audit-loading-more {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 12px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.audit-end-hint {
  text-align: center;
  font-size: 12px;
  color: var(--td-text-color-disabled);
  padding: 8px 0 14px;
  margin: 0;
}

.audit-time {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.3;

  .audit-time-date {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }

  .audit-time-clock {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    font-variant-numeric: tabular-nums;
  }
}

.audit-actor {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.3;
  min-width: 0;

  .audit-actor-name {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .audit-actor-role {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }
}

.audit-target {
  display: flex;
  flex-direction: column;
  gap: 4px;
  line-height: 1.35;
  min-width: 0;
  padding: 2px 0;

  .audit-target-key {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    word-break: break-all;
    font-family: var(--td-font-family-mono, monospace);
  }

  .audit-target-diff {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    font-family: var(--td-font-family-mono, monospace);
    word-break: break-all;
    line-height: 1.4;
  }

  .audit-target-empty {
    color: var(--td-text-color-placeholder);
  }
}

/* Expanded row: surfaces the raw audit row context (UUIDs, target
   type/id, full details JSON) so an investigator never has to hop to
   psql for the verbatim event. Background steps off-card to make the
   nested context visually distinct from the table rows. */
.audit-expanded {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px 16px;
  background: var(--td-bg-color-container-hover);
}

.audit-expanded-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 10px 18px;
}

.audit-expanded-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.audit-expanded-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--td-text-color-secondary);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.audit-expanded-value {
  font-size: 12px;
  color: var(--td-text-color-primary);
  word-break: break-all;
}

.audit-expanded-details {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.audit-expanded-json {
  margin: 0;
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.55;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 280px;
  overflow: auto;
}

.mono {
  font-family: var(--td-font-family-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
}

.data-table-shell {
  overflow-x: auto;
  border-radius: 10px;
  border: 1px solid var(--td-component-stroke);
  background-color: var(--td-bg-color-container);

  &:deep(thead th) {
    font-weight: 600;
    font-size: 13px;
    background-color: var(--td-bg-color-secondarycontainer) !important;
  }

  &:deep(.t-table td),
  &:deep(.t-table th) {
    padding-top: 14px;
    padding-bottom: 14px;
    /* Center the cell content vertically: most rows have at least one
       single-line tag column (action / outcome), and a top-aligned
       layout floats those chips above the multi-line target cell —
       middle keeps the row's visual weight unified. */
    vertical-align: middle;
  }
}

/* Audit-specific table polish: no zebra stripes (the per-row "key /
   diff" stack already provides enough separation between rows; stripes
   on top read as visual noise), softer hover, denser separator. */
.audit-table-shell {
  /* Sticky table head: long audit feeds (50+ rows) lose the column
     labels once the user scrolls, which makes "what's this column?"
     a constant relearn. The drawer's outer scroll container is
     `.audit-scroll-area`, so `top: 0` here pins thead to that
     container's top. z-index keeps it above row hover/expand
     backgrounds, and the explicit background plus bottom border
     prevent row content bleeding through during scroll. */
  &:deep(thead th) {
    position: sticky;
    top: 0;
    z-index: 2;
    box-shadow: inset 0 -1px 0 var(--td-component-stroke);
  }

  &:deep(.t-table tbody tr:hover > td) {
    background-color: var(--td-bg-color-container-hover);
  }

  &:deep(.t-table tbody tr.t-table__expanded-row > td) {
    padding: 0 !important;
    background-color: transparent;
  }

  &:deep(.t-table__expandable-icon-cell) {
    width: 36px;
  }
}

.priority-hint {
  margin-bottom: 24px;
  padding: 14px 16px;
  border-radius: 6px;
  background: var(--td-bg-color-container-hover);
  border-left: 3px solid var(--td-brand-color);
}

.priority-hint-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.priority-hint-icon {
  color: var(--td-brand-color);
  font-size: 16px;
}

.priority-hint-title {
  font-size: 14px;
  font-weight: 500;
  color: var(--td-text-color-primary);
}

.priority-hint-list {
  margin: 4px 0 0;
  padding-left: 22px;
  font-size: 13px;
  line-height: 1.65;
  color: var(--td-text-color-primary);
  list-style: disc;

  li + li {
    margin-top: 4px;
  }
}

.setting-reset-btn {
  // Sit flush with the input on the right; size="small" gives it the
  // right footprint to read as secondary action next to the primary
  // edit control.
  flex-shrink: 0;
}

// Anchor wrapper for inline t-popconfirm on inputs (SSRF / admins /
// high-risk select). Popconfirm attaches to this box so the bubble
// appears beside the control, not a full-screen modal.
.setting-control-anchor {
  flex: 1;
  min-width: 0;
}

.loading-state,
.empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 60px 0;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
}

.empty-state--group {
  justify-content: flex-start;
  padding: 18px 0;
}

// Skeleton mirrors GeneralSettings.vue 1:1 so the two panes feel like
// they came from the same hand. Values that diverge intentionally:
//   - .setting-label is a flex container (vs General's plain <label>)
//     because we render badges inline with the title; identical font /
//     spacing otherwise.
//   - .desc has a max-width so long backend descriptions don't push
//     the control off the canvas in narrow viewports.
.settings-group {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 20px 0;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
  }
}

.setting-info {
  flex: 1;
  max-width: 65%;
  padding-right: 24px;
}

.setting-label {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  font-size: 15px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  margin-bottom: 4px;
  line-height: 1.4;
}

.setting-badge {
  vertical-align: middle;
}

.desc {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  margin: 0;
  line-height: 1.5;
  max-width: 480px;
}

.setting-meta {
  margin-top: 6px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.setting-control {
  flex-shrink: 0;
  min-width: 280px;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
}

.setting-control-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.setting-control-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
  flex-wrap: wrap;
}

.setting-saving {
  // Pin width so the row layout doesn't reflow when the spinner
  // appears / disappears mid-save.
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.setting-input {
  width: 240px;
}

.setting-input--wide {
  width: 320px;
}

@media (max-width: 860px) {
  .setting-row {
    flex-direction: column;
    gap: 12px;
  }

  .setting-control {
    width: 100%;
    align-items: flex-start;
  }

  .setting-control-row {
    width: 100%;
    justify-content: flex-start;
  }

  .setting-control-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .setting-input,
  .setting-input--wide {
    width: 100%;
    flex: 1;
  }

  .desc {
    max-width: none;
  }
}

@media (max-width: 760px) {
  .system-settings-container {
    flex-direction: column;
  }

  .system-settings-sidebar {
    width: 100%;
    max-height: 248px;
    border-right: none;
    border-bottom: 1px solid var(--td-component-stroke);
  }

  .system-settings-content-header {
    padding-right: 52px;
  }
}
</style>

<style lang="less">
/* t-drawer teleports its content-wrapper to body, so the height-chain
   needed for the internal scroll area must be declared globally. Same
   pattern as `.tenant-members-audit-drawer` in TenantMembers.vue. */
.t-drawer.system-settings-audit-drawer.t-drawer--right .t-drawer__content-wrapper--right {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  max-height: 100vh;
  height: 100%;
}

.t-drawer.system-settings-audit-drawer .t-drawer__body {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  overflow: hidden !important;
}
</style>
