<script lang="ts">
  import { createEventDispatcher } from 'svelte'

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

<button
  type="button"
  class={`proxy-inline-switch ${checked ? 'is-checked' : ''}`}
  aria-checked={checked}
  role="switch"
  on:click={handleToggle}
  {disabled}
>
  <span class="proxy-inline-switch-thumb" />
</button>

<style>
  .proxy-inline-switch {
    position: relative;
    display: inline-flex;
    align-items: center;
    width: 2.95rem;
    height: 1.72rem;
    padding: 0.16rem;
    border-radius: 999px;
    border: 1px solid color-mix(in srgb, var(--color-border) 88%, white 6%);
    background:
      linear-gradient(180deg, color-mix(in srgb, white 4%, transparent), transparent),
      color-mix(in srgb, var(--color-app) 82%, var(--color-surface));
    transition:
      border-color 0.2s ease,
      background-color 0.2s ease,
      box-shadow 0.2s ease;
    box-shadow:
      inset 0 1px 0 color-mix(in srgb, white 5%, transparent),
      inset 0 -1px 0 color-mix(in srgb, black 12%, transparent);
  }

  .proxy-inline-switch.is-checked {
    border-color: color-mix(in srgb, #f59e0b 55%, var(--color-border));
    background:
      linear-gradient(180deg, color-mix(in srgb, white 4%, transparent), transparent),
      color-mix(in srgb, #f59e0b 18%, var(--color-app));
    box-shadow:
      inset 0 1px 0 color-mix(in srgb, white 7%, transparent),
      0 0 0 1px color-mix(in srgb, #f59e0b 18%, transparent);
  }

  .proxy-inline-switch:disabled {
    cursor: not-allowed;
    opacity: 0.55;
  }

  .proxy-inline-switch-thumb {
    width: 1.18rem;
    height: 1.18rem;
    border-radius: 999px;
    background: linear-gradient(180deg, color-mix(in srgb, white 92%, transparent), color-mix(in srgb, white 74%, var(--color-surface)));
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.45),
      0 8px 16px rgba(0, 0, 0, 0.22);
    transition: transform 0.22s ease;
    transform: translateX(0);
  }

  .proxy-inline-switch.is-checked .proxy-inline-switch-thumb {
    transform: translateX(1.08rem);
  }
</style>
