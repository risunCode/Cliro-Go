<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import ModalBackdrop from '@/components/common/ModalBackdrop.svelte'

  export let open = false
  export let overlayClass = 'items-center justify-center p-4'
  export let cardClass = 'w-full max-w-lg overflow-hidden'
  export let headerClass = 'border-b border-border px-5 py-4'
  export let bodyClass = 'px-5 py-4'
  export let footerClass = 'border-t border-border px-5 py-4'

  const dispatch = createEventDispatcher<{ close: void }>()

  const handleClose = (): void => {
    dispatch('close')
  }
</script>

{#if open}
  <ModalBackdrop on:close={handleClose} />

  <div class={`fixed inset-0 z-50 flex ${overlayClass}`}>
    <div class={`ui-surface-card ${cardClass}`}>
      {#if $$slots.header}
        <header class={headerClass}>
          <slot name="header" />
        </header>
      {/if}

      <div class={bodyClass}>
        <slot />
      </div>

      {#if $$slots.footer}
        <footer class={footerClass}>
          <slot name="footer" />
        </footer>
      {/if}
    </div>
  </div>
{/if}
