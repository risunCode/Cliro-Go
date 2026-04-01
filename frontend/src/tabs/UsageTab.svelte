<script lang="ts">
  import type { AppState, LogEntry } from '@/app/types'
  import UsageWorkspace from '@/features/usage/components/UsageWorkspace.svelte'
  import type { Account } from '@/features/accounts/types'
  import type { ProxyStatus } from '@/features/router/types'

  export let state: AppState | null = null
  export let accounts: Account[] = []
  export let proxyStatus: ProxyStatus | null = null
  export let logs: LogEntry[] = []
</script>

<UsageWorkspace {state} {accounts} {proxyStatus} {logs} />
