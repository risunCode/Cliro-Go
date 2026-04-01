<script lang="ts">
  import type { AppActions, AppShellState, AccountsActions, LogsActions, RouterActions, SettingsActions } from '@/app/services/app-controller'
  import type { AppTabId } from '@/app/lib/tabs'
  import AppHeader from '@/components/common/AppHeader.svelte'
  import AppFooter from '@/components/common/AppFooter.svelte'
  import DashboardTab from '@/tabs/DashboardTab.svelte'
  import AccountsTab from '@/tabs/AccountsTab.svelte'
  import ApiRouterTab from '@/tabs/ApiRouterTab.svelte'
  import UsageTab from '@/tabs/UsageTab.svelte'
  import SystemLogsTab from '@/tabs/SystemLogsTab.svelte'
  import SettingsTab from '@/tabs/SettingsTab.svelte'
  import type { Theme } from '@/shared/stores/theme'

  export let shell: AppShellState
  export let theme: Theme = 'light'
  export let appActions: AppActions
  export let accountsActions: AccountsActions
  export let routerActions: RouterActions
  export let logsActions: LogsActions
  export let settingsActions: SettingsActions
  export let onToggleTheme: () => void

  const handleTabChange = (tabId: AppTabId): void => {
    appActions.setActiveTab(tabId)
  }
</script>

<div class="flex h-full flex-col">
  <AppHeader activeTab={shell.activeTab} onSelectTab={handleTabChange} {onToggleTheme} {theme} />

  <section class="no-scrollbar min-h-0 flex-1 overflow-y-auto px-4 py-4 md:px-6">
    <div class="space-y-4 pb-1">
      {#if shell.activeTab === 'dashboard'}
        <DashboardTab state={shell.state} accounts={shell.accounts} proxyStatus={shell.proxyStatus} loading={shell.loadingDashboard} />
      {:else if shell.activeTab === 'accounts'}
        <AccountsTab shell={shell} {appActions} {accountsActions} />
      {:else if shell.activeTab === 'api-router'}
        <ApiRouterTab proxyStatus={shell.proxyStatus} busy={shell.proxyBusy} {routerActions} />
      {:else if shell.activeTab === 'usage'}
        <UsageTab state={shell.state} accounts={shell.accounts} proxyStatus={shell.proxyStatus} logs={shell.logs} />
      {:else if shell.activeTab === 'system-logs'}
        <SystemLogsTab shell={shell} {logsActions} />
      {:else if shell.activeTab === 'settings'}
        <SettingsTab {settingsActions} />
      {/if}
    </div>
  </section>

  <AppFooter proxyStatus={shell.proxyStatus} state={shell.state} loading={shell.loadingDashboard} />
</div>
