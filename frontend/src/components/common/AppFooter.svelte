<script lang="ts">
  import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
  import type { ProxyStatus } from '@/services/wails-api-types'

  const repoURL = 'https://github.com/risunCode/Cliro-Go'
  export let proxyStatus: ProxyStatus | null = null

  const openURL = (event: MouseEvent, url: string): void => {
    event.preventDefault()
    BrowserOpenURL(url)
  }

  $: serviceAddress = proxyStatus
    ? `${proxyStatus.allowLan ? '0.0.0.0' : '127.0.0.1'}:${proxyStatus.port}`
    : '127.0.0.1:8095'
</script>

<footer class="rounded-t-base border border-border bg-surface px-4 py-3 text-xs text-text-secondary shadow-soft md:px-6">
  <div class="flex flex-col gap-2 md:flex-row md:items-center md:justify-between md:gap-4">
    <p>
      Service status: {proxyStatus?.running ? 'Online' : 'Offline'} at port:
      <code class="ml-1 rounded-sm bg-app px-1.5 py-0.5 text-[11px] text-text-primary">{serviceAddress}</code>
    </p>

    <a
      href={repoURL}
      class="inline-flex w-fit items-center gap-1 rounded-sm border border-transparent px-2 py-1 text-text-secondary transition hover:border-border hover:bg-app hover:text-text-primary"
      on:click={(event) => openURL(event, repoURL)}
    >
      CLIrouter Github
    </a>
  </div>
</footer>
