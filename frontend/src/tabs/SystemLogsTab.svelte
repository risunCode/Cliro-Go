<script lang="ts">
  import type { AppShellState, LogsActions } from '@/app/services/app-controller'
  import SystemLogsWorkspace from '@/features/logs/components/SystemLogsWorkspace.svelte'

  export let shell: AppShellState
  export let logsActions: LogsActions
</script>

<SystemLogsWorkspace
  logs={shell.logs}
  loading={shell.loadingLogs}
  clearing={shell.clearingLogs}
  onRefreshLogs={logsActions.refresh}
  onClearLogs={logsActions.clear}
/>
