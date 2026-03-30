<script lang="ts">
  import { fly, scale } from 'svelte/transition'
  import { toastStore, type ToastType } from '@/stores/toast'
  import Button from '@/components/common/Button.svelte'
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'

  const labels: Record<ToastType, string> = {
    success: 'Success',
    error: 'Error',
    info: 'Info',
    warning: 'Warning'
  }

  const dismiss = (id: number): void => {
    toastStore.remove(id)
  }

</script>

<div
  aria-atomic="true"
  aria-live="polite"
  class="toast-viewport pointer-events-none fixed bottom-4 right-4 z-[70] flex w-[20rem] max-w-[calc(100vw-1.5rem)] flex-col gap-2.5 sm:bottom-6 sm:right-6"
>
  {#each $toastStore as toast (toast.id)}
    <div
      class="pointer-events-auto"
      in:fly={{ y: 12, duration: 210, opacity: 0 }}
      out:scale={{ duration: 170, start: 1, opacity: 0 }}
    >
    <SurfaceCard
      as="article"
      className="toast-card relative overflow-hidden border p-2.5 shadow-soft"
      data-type={toast.type}
      role="status"
      style={`--toast-duration: ${toast.duration}ms`}
    >
      <span class="toast-accent" aria-hidden="true"></span>

      <div class="mb-1.5 flex items-start justify-between gap-2.5 pl-2.5">
        <div class="min-w-0">
          <p class="toast-type text-[0.7rem] font-semibold uppercase tracking-[0.08em]">{labels[toast.type]}</p>
          <p class="mt-0.5 text-sm font-semibold leading-tight text-text-primary">{toast.title}</p>
        </div>

        <Button
          aria-label="Dismiss notification"
          className="shrink-0 !px-1.5 !py-0.5 !text-[0.7rem] !font-semibold"
          on:click={() => dismiss(toast.id)}
          size="sm"
          variant="ghost"
        >
          Close
        </Button>
      </div>

      <p class="pl-2.5 text-xs leading-snug text-text-secondary">{toast.message}</p>

      <div class="toast-progress-track mt-2.5" aria-hidden="true">
        <div class="toast-progress"></div>
      </div>
    </SurfaceCard>
    </div>
  {/each}
</div>
