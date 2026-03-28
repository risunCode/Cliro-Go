<script lang="ts">
  import { FolderCog, FolderOpen } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'

  export let onOpenDataDir: () => Promise<void>

  const dataDirPath = '~/.cliro-go'
  const configPath = '~/.cliro-go/config.json'
</script>

<div class="space-y-4">
  <SurfaceCard className="p-4">
    <p class="text-sm font-semibold text-text-primary">Settings</p>
    <p class="mt-1 text-xs text-text-secondary">Proxy operational controls are centralized in API Router.</p>
  </SurfaceCard>

  <SurfaceCard className="p-4">
    <div class="mb-2 flex items-center justify-between gap-2">
      <div class="flex items-center gap-2">
        <FolderCog size={15} class="text-text-secondary" />
        <p class="text-sm font-semibold text-text-primary">Data Path</p>
      </div>
      <Button variant="secondary" size="sm" on:click={onOpenDataDir}>
        <FolderOpen size={13} class="mr-1" />
        Open Folder
      </Button>
    </div>
    <p class="mb-2 rounded-sm border border-border bg-app p-2 font-mono text-xs text-text-secondary">{dataDirPath}</p>
    <p class="rounded-sm border border-border bg-app p-2 font-mono text-xs text-text-secondary">{configPath}</p>
    <p class="mt-2 text-xs text-text-secondary">Primary config file used by backend state manager.</p>
  </SurfaceCard>
</div>
