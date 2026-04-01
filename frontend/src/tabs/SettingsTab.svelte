<script lang="ts">
  import { Database, FolderOpen, Upload } from 'lucide-svelte'
  import type { SettingsActions } from '@/app/services/app-controller'
  import { validateBackupPayload } from '@/app/lib/backup'
  import Button from '@/components/common/Button.svelte'
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'
  import { createAsyncTaskState, runAsyncTask, type AsyncTaskState } from '@/shared/lib/async'

  export let settingsActions: SettingsActions

  let backupFileInput: HTMLInputElement | null = null
  let task: AsyncTaskState = createAsyncTaskState()
  let statusMessage = ''
  $: busy = task.busy
  $: errorMessage = task.error

  const setBusy = async (action: () => Promise<void>, successMessage = 'Settings saved successfully.'): Promise<void> => {
    statusMessage = ''

    try {
      await runAsyncTask((nextState) => {
        task = nextState
      }, action)
      statusMessage = successMessage
    } catch {
      statusMessage = ''
    }
  }

  const handleRestoreFromFile = async (event: Event): Promise<void> => {
    const target = event.currentTarget as HTMLInputElement
    const file = target.files?.[0]
    if (!file) return

    await setBusy(
      async () => {
        const text = await file.text()
        const parsedPayload = JSON.parse(text) as unknown
        const payload = validateBackupPayload(parsedPayload)

        await settingsActions.restoreBackup(payload, (progress) => {
          statusMessage = `Restoring step ${progress.index}/${progress.total}: ${progress.step}`
        })
      },
      'Backup restored successfully.'
    )

    target.value = ''
  }

  const handleExportBackup = async (): Promise<void> => {
    await setBusy(async () => {
      await settingsActions.exportBackup()
    }, 'Backup exported successfully.')
  }

  const dataDirPath = '~/.cliro-go'
</script>

<div class="settings-tab space-y-2.5">
  <!-- Data Folder + Backup Tools -->
  <div class="grid gap-2.5 lg:grid-cols-2">
    <SurfaceCard className="p-3.5">
      <div class="mb-3 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <Database size={15} class="text-text-secondary" />
          <p class="text-sm font-semibold text-text-primary">Data Folder</p>
        </div>
        <Button variant="secondary" size="sm" on:click={() => void settingsActions.openDataDir()} disabled={busy}>
          <FolderOpen size={13} class="mr-1" />
          Open
        </Button>
      </div>

      <div class="rounded border border-border bg-app p-2 font-mono text-xs text-text-secondary">{dataDirPath}</div>
    </SurfaceCard>

    <SurfaceCard className="p-3.5">
      <div class="mb-3 flex items-center gap-2">
        <Upload size={15} class="text-text-secondary" />
        <p class="text-sm font-semibold text-text-primary">Backup Tools</p>
      </div>

      <div class="flex flex-wrap gap-2">
        <Button variant="secondary" size="sm" on:click={() => void handleExportBackup()} disabled={busy}>
          <Database size={13} class="mr-1" />
          Export Backup
        </Button>
        <Button
          variant="secondary"
          size="sm"
          on:click={() => {
            backupFileInput?.click()
          }}
          disabled={busy}
        >
          <Upload size={13} class="mr-1" />
          Restore Backup
        </Button>
      </div>

      {#if statusMessage}
        <p class="mt-2 text-xs text-success">{statusMessage}</p>
      {/if}
      {#if errorMessage}
        <p class="mt-2 text-xs text-error">{errorMessage}</p>
      {/if}

      <input bind:this={backupFileInput} type="file" accept=".json,application/json" class="hidden" on:change={handleRestoreFromFile} />
    </SurfaceCard>
  </div>
</div>
