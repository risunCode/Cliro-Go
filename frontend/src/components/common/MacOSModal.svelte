<script lang="ts">
  import { createEventDispatcher, onDestroy } from 'svelte'
  import { fade, scale } from 'svelte/transition'
  import { quintOut } from 'svelte/easing'

  export let open = false
  export let title = 'Modal'
  export let size: 'sm' | 'md' | 'lg' | 'xl' = 'lg'
  export let closeOnBackdrop = true
  export let closeOnEscape = true

  const dispatch = createEventDispatcher<{ close: void }>()
  let showSymbols = false

  const sizeClass = {
    sm: 'modal-sm',
    md: 'modal-md',
    lg: 'modal-lg',
    xl: 'modal-xl'
  }[size]

  const closeModal = (): void => {
    dispatch('close')
  }

  const handleBackdropClick = (event: MouseEvent): void => {
    if (!closeOnBackdrop) {
      return
    }
    if (event.target === event.currentTarget) {
      closeModal()
    }
  }

  const handleBackdropKeydown = (event: KeyboardEvent): void => {
    if (!closeOnBackdrop || event.key !== 'Escape') {
      return
    }
    event.preventDefault()
    closeModal()
  }

  const handleKeydown = (event: KeyboardEvent): void => {
    if (!open || !closeOnEscape || event.key !== 'Escape') {
      return
    }
    event.preventDefault()
    closeModal()
  }

  $: if (typeof document !== 'undefined') {
    document.body.style.overflow = open ? 'hidden' : ''
  }

  onDestroy(() => {
    if (typeof document !== 'undefined') {
      document.body.style.overflow = ''
    }
  })
</script>

<svelte:window on:keydown={handleKeydown} />

{#if open}
  <div
    class="macos-modal-backdrop"
    role="presentation"
    tabindex="-1"
    on:click={handleBackdropClick}
    on:keydown={handleBackdropKeydown}
    transition:fade={{ duration: 180, easing: quintOut }}
  >
    <div class={`macos-modal-shell ${sizeClass}`} transition:scale={{ duration: 180, start: 0.97, easing: quintOut }} role="dialog" aria-modal="true" aria-label={title}>
      <div class="macos-modal-titlebar" on:mouseenter={() => (showSymbols = true)} on:mouseleave={() => (showSymbols = false)}>
        <div class="macos-traffic-lights">
          <button type="button" class="macos-traffic-light is-red" aria-label="Close" on:click={closeModal}>
            {#if showSymbols}
              <span class="macos-traffic-symbol">x</span>
            {/if}
          </button>
          <button type="button" class="macos-traffic-light is-yellow" aria-label="Minimize" disabled>
            {#if showSymbols}
              <span class="macos-traffic-symbol">-</span>
            {/if}
          </button>
          <button type="button" class="macos-traffic-light is-green" aria-label="Maximize" disabled>
            {#if showSymbols}
              <span class="macos-traffic-symbol">+</span>
            {/if}
          </button>
        </div>

        <div class="macos-modal-title">{title}</div>

        <div class="macos-modal-title-actions">
          <slot name="titleActions" />
        </div>
      </div>

      <div class="macos-modal-content">
        <slot />
      </div>

      {#if $$slots.footer}
        <div class="macos-modal-footer">
          <slot name="footer" />
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .macos-modal-backdrop {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 12px;
    background: var(--modal-overlay);
    backdrop-filter: blur(var(--modal-overlay-blur));
  }

  .macos-modal-shell {
    width: 100%;
    max-height: 92vh;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    border-radius: 12px;
    border: 1px solid rgba(15, 23, 42, 0.2);
    background: #0f172a;
    box-shadow: 0 24px 60px rgba(2, 6, 23, 0.55);
  }

  .modal-sm {
    max-width: 420px;
  }

  .modal-md {
    max-width: 620px;
  }

  .modal-lg {
    max-width: 860px;
  }

  .modal-xl {
    max-width: 1080px;
  }

  .macos-modal-titlebar {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 44px;
    padding: 10px 52px;
    border-bottom: 1px solid rgba(15, 23, 42, 0.25);
    background: linear-gradient(180deg, #aeb3ba 0%, #8f959d 100%);
  }

  .macos-modal-title {
    font-size: 13px;
    font-weight: 700;
    color: #111827;
    letter-spacing: 0.01em;
    user-select: none;
  }

  .macos-traffic-lights {
    position: absolute;
    left: 12px;
    top: 50%;
    transform: translateY(-50%);
    display: flex;
    gap: 8px;
  }

  .macos-traffic-light {
    width: 12px;
    height: 12px;
    border-radius: 999px;
    border: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    cursor: pointer;
  }

  .macos-traffic-light:disabled {
    cursor: default;
  }

  .macos-traffic-light.is-red {
    background: #ff5f57;
  }

  .macos-traffic-light.is-yellow {
    background: #ffbd2e;
  }

  .macos-traffic-light.is-green {
    background: #28c840;
  }

  .macos-traffic-symbol {
    font-size: 9px;
    font-weight: 700;
    color: rgba(15, 23, 42, 0.75);
    line-height: 1;
  }

  .macos-modal-title-actions {
    position: absolute;
    right: 10px;
    top: 50%;
    transform: translateY(-50%);
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .macos-modal-content {
    flex: 1;
    overflow: auto;
    background: #111827;
    color: #d1d5db;
  }

  .macos-modal-footer {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 8px;
    padding: 10px 12px 12px;
    border-top: 1px solid rgba(255, 255, 255, 0.08);
    background: #111827;
  }
</style>
