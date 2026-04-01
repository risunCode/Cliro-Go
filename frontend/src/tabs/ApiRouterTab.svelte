<script lang="ts">
  import { onMount } from 'svelte'
  import type { RouterActions } from '@/app/services/app-controller'
  import CliSyncPanel from '@/features/router/components/cli-sync/CliSyncPanel.svelte'
  import CloudflaredPanel from '@/features/router/components/cloudflared/CloudflaredPanel.svelte'
  import EndpointTesterPanel from '@/features/router/components/endpoint-tester/EndpointTesterPanel.svelte'
  import ModelAliasPanel from '@/features/router/components/model-alias/ModelAliasPanel.svelte'
  import ProxyControlsPanel from '@/features/router/components/proxy/ProxyControlsPanel.svelte'
  import SchedulingPanel from '@/features/router/components/scheduling/SchedulingPanel.svelte'
  import type { ProxyStatus } from '@/features/router/types'

  export let proxyStatus: ProxyStatus | null = null
  export let busy = false
  export let routerActions: RouterActions

  onMount(() => {
    void routerActions.refreshProxyStatus().catch(() => {})
  })
</script>

<div class="space-y-4">
  <ProxyControlsPanel
    {proxyStatus}
    {busy}
    onStartProxy={routerActions.startProxy}
    onStopProxy={routerActions.stopProxy}
    onSetProxyPort={routerActions.setProxyPort}
    onSetAllowLAN={routerActions.setAllowLAN}
    onSetAutoStartProxy={routerActions.setAutoStartProxy}
    onSetProxyAPIKey={routerActions.setProxyAPIKey}
    onRegenerateProxyAPIKey={routerActions.regenerateProxyAPIKey}
    onSetAuthorizationMode={routerActions.setAuthorizationMode}
  />

  <CloudflaredPanel
    {proxyStatus}
    {busy}
    onRefreshCloudflaredStatus={routerActions.refreshCloudflaredStatus}
    onSetCloudflaredConfig={routerActions.setCloudflaredConfig}
    onInstallCloudflared={routerActions.installCloudflared}
    onStartCloudflared={routerActions.startCloudflared}
    onStopCloudflared={routerActions.stopCloudflared}
  />

  <CliSyncPanel
    {busy}
    proxyBaseURL={proxyStatus?.url || ''}
    proxyAPIKey={proxyStatus?.proxyApiKey || ''}
    onGetCLISyncStatuses={routerActions.getCliSyncStatuses}
    onGetCLISyncFileContent={routerActions.getCliSyncFileContent}
    onSaveCLISyncFileContent={routerActions.saveCliSyncFileContent}
    onSyncCLIConfig={routerActions.syncCLIConfig}
  />

  <EndpointTesterPanel proxyStatus={proxyStatus} apiKey={proxyStatus?.proxyApiKey || ''} />

  <ModelAliasPanel
    {busy}
    onGetModelAliases={routerActions.getModelAliases}
    onSetModelAliases={routerActions.setModelAliases}
  />

  <SchedulingPanel
    {proxyStatus}
    {busy}
    onSetSchedulingMode={routerActions.setSchedulingMode}
  />
</div>
