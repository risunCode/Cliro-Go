<script lang="ts">
  import { Clock3, Database, FolderOpen, Save, Upload } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'
  import type { Account, AppState } from '@/services/wails-api'

  interface BackupPayload {
    version: number
    exportedAt: string
    state: AppState | null
    accounts: Account[]
  }

  interface RestoreProgress {
    step: string
    index: number
    total: number
  }

  export let state: AppState | null = null
  export let onOpenDataDir: () => Promise<void>
  export let currentAutoRefreshMinutes = 5
  export let onSetSessionAutoRefreshMinutes: (minutes: number) => void
  export let onExportBackup: () => Promise<void>
  export let onRestoreBackup: (payload: BackupPayload, onProgress?: (progress: RestoreProgress) => void) => Promise<void>

  let autoRefreshPresetInput = '5'

  let backupFileInput: HTMLInputElement | null = null
  let busy = false
  let statusMessage = ''
  let errorMessage = ''

  const setBusy = async (action: () => Promise<void>, successMessage = 'Settings saved successfully.'): Promise<void> => {
    busy = true
    errorMessage = ''
    statusMessage = ''
    try {
      await action()
      statusMessage = successMessage
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : 'Operation failed.'
    } finally {
      busy = false
    }
  }

  const parseIntWithBounds = (value: string, fallback: number, minValue: number, maxValue: number): number => {
    const parsed = Number.parseInt(value.trim(), 10)
    if (!Number.isFinite(parsed)) return fallback
    if (parsed < minValue) return minValue
    if (parsed > maxValue) return maxValue
    return parsed
  }

  const isRecord = (value: unknown): value is Record<string, unknown> => {
    return typeof value === 'object' && value !== null
  }

  const validateBackupPayload = (value: unknown): BackupPayload => {
    if (!isRecord(value)) throw new Error('Backup payload must be a JSON object.')

    const version = Number(value.version)
    if (!Number.isFinite(version) || version <= 0) throw new Error('Backup payload version is invalid.')

    const rawState = value.state
    const state = rawState === null || rawState === undefined ? null : (isRecord(rawState) ? (rawState as unknown as AppState) : null)
    if (rawState !== null && rawState !== undefined && !isRecord(rawState)) {
      throw new Error('Backup payload state must be an object or null.')
    }

    if (!Array.isArray(value.accounts)) throw new Error('Backup payload accounts must be an array.')

    const accounts = value.accounts.filter((entry) => isRecord(entry)) as unknown as Account[]
    const exportedAt = typeof value.exportedAt === 'string' ? value.exportedAt : new Date().toISOString()

    return { version, exportedAt, state, accounts }
  }

  const handleApplyAutoRefresh = async (): Promise<void> => {
    const minutes = parseIntWithBounds(autoRefreshPresetInput, currentAutoRefreshMinutes || 5, 1, 1440)
    onSetSessionAutoRefreshMinutes(minutes)
    statusMessage = `Auto refresh now checks the last-used eligible account every ${minutes} minute(s).`
    errorMessage = ''
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

        await onRestoreBackup(payload, (progress) => {
          statusMessage = `Restoring step ${progress.index}/${progress.total}: ${progress.step}`
        })
      },
      'Backup restored successfully.'
    )

    target.value = ''
  }

  const handleExportBackup = async (): Promise<void> => {
    await setBusy(async () => {
      await onExportBackup()
    }, 'Backup exported successfully.')
  }

  $: if (state) {
    autoRefreshPresetInput = String(Math.max(1, Number(currentAutoRefreshMinutes || state.autoRefreshMinutes || 5)))
  }

  const dataDirPath = '~/.cliro-go'
</script>

<div class="settings-tab space-y-2.5">
  <SurfaceCard className="p-3.5">
    <div class="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
      <div>
        <p class="text-sm font-semibold text-text-primary">Settings</p>
        <p class="mt-0.5 text-xs text-text-secondary">Compact controls for refresh behavior, local data, and safety tools.</p>
      </div>
      <Button variant="primary" size="sm" on:click={() => void handleApplyAutoRefresh()} disabled={busy}>
        <Save size={14} class="mr-1" />
        Apply Refresh Rule
      </Button>
    </div>
    {#if statusMessage}
      <p class="mt-2 text-xs text-success">{statusMessage}</p>
    {/if}
    {#if errorMessage}
      <p class="mt-2 text-xs text-error">{errorMessage}</p>
    {/if}
  </SurfaceCard>

  <div class="grid gap-2.5">
    <SurfaceCard className="p-3.5">
      <div class="mb-3 flex items-center gap-2">
          <Clock3 size={15} class="text-text-secondary" />
          <p class="text-sm font-semibold text-text-primary">Auto Refresh</p>
      </div>

      <div class="grid gap-2 md:grid-cols-[160px_minmax(0,1fr)] md:items-start">
        <label class="settings-field">
            <span>Interval</span>
            <select bind:value={autoRefreshPresetInput} class="ui-control-input ui-control-select-sm bg-app px-2" disabled={busy}>
              <option value="1">1 minute</option>
              <option value="3">3 minutes</option>
              <option value="5">5 minutes</option>
              <option value="10">10 minutes</option>
              <option value="15">15 minutes</option>
            </select>
        </label>

        <div class="rounded border border-border bg-app p-2.5 text-xs text-text-secondary">
          <p class="font-semibold text-text-primary">New Logic</p>
          <p class="mt-1">Refresh checks the last-used account only.</p>
          <p class="mt-1">Skipped automatically: breached/banned, disabled, and quota-exhausted accounts that are still cooling down.</p>
          <p class="mt-1">Sequential only, no parallel quota refresh.</p>
        </div>
      </div>
    </SurfaceCard>

    <div class="grid gap-2.5 lg:grid-cols-2">
      <SurfaceCard className="p-3.5">
        <div class="mb-3 flex items-center justify-between">
          <div class="flex items-center gap-2">
            <Database size={15} class="text-text-secondary" />
            <p class="text-sm font-semibold text-text-primary">Data Folder</p>
          </div>
          <Button variant="secondary" size="sm" on:click={() => void onOpenDataDir()} disabled={busy}>
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

        <input bind:this={backupFileInput} type="file" accept=".json,application/json" class="hidden" on:change={handleRestoreFromFile} />
      </SurfaceCard>
    </div>
  </div>
</div>
