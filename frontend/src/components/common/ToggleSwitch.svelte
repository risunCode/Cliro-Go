<script lang="ts">
  import { createEventDispatcher } from 'svelte'

  export let label = ''
  export let checked = false
  export let disabled = false

  const dispatch = createEventDispatcher<{ change: boolean }>()

  const handleToggle = (): void => {
    if (disabled) {
      return
    }

    checked = !checked
    dispatch('change', checked)
  }
</script>

<div class="flex items-center justify-between gap-3 rounded-sm border border-border bg-app px-3 py-2 text-xs text-text-secondary">
  <span>{label}</span>
  <button
    type="button"
    class="toggle-track relative h-6 w-11 rounded-full border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-text-primary focus-visible:ring-offset-2 focus-visible:ring-offset-app disabled:cursor-not-allowed disabled:opacity-50"
    class:border-border={!checked}
    class:bg-surface={!checked}
    class:toggle-track-checked={checked}
    aria-checked={checked}
    role="switch"
    on:click={handleToggle}
    {disabled}
  >
    <span
      class="toggle-thumb absolute left-0.5 top-1/2 h-4 w-4 -translate-y-1/2 rounded-full transition-transform"
      class:translate-x-0={!checked}
      class:translate-x-6={checked}
    />
  </button>
</div>

<style>
  .toggle-track-checked {
    border-color: color-mix(in srgb, var(--accent-primary) 70%, var(--color-border));
    background-color: color-mix(in srgb, var(--accent-primary) 42%, var(--color-surface));
  }

  .toggle-thumb {
    background-color: color-mix(in srgb, var(--color-surface) 84%, var(--color-text-primary));
  }

  .toggle-track-checked .toggle-thumb {
    background-color: color-mix(in srgb, var(--color-bg) 88%, var(--color-surface));
  }
</style>
