<script lang="ts">
  import { createEventDispatcher, onDestroy } from 'svelte'
  import ModalBackdrop from '@/components/common/ModalBackdrop.svelte'

  export let open = false
  export let overlayClass = 'items-center justify-center p-4'
  export let cardClass = 'w-full max-w-lg overflow-hidden'
  export let headerClass = 'border-b border-border px-5 py-4'
  export let bodyClass = 'px-5 py-4'
  export let footerClass = 'border-t border-border px-5 py-4'

  const dispatch = createEventDispatcher<{ close: void }>()

  let scrollLocked = false
  let originalDocumentOverflow = ''
  let originalOverflow = ''
  let originalPaddingRight = ''

  const portal = (node: HTMLElement) => {
    if (typeof document === 'undefined') {
      return {}
    }

    document.body.appendChild(node)

    return {
      destroy() {
        if (node.parentNode) {
          node.parentNode.removeChild(node)
        }
      }
    }
  }

  const lockScroll = (): void => {
    if (scrollLocked || typeof document === 'undefined') {
      return
    }

    const scrollbarWidth = window.innerWidth - document.documentElement.clientWidth
    originalDocumentOverflow = document.documentElement.style.overflow
    originalOverflow = document.body.style.overflow
    originalPaddingRight = document.body.style.paddingRight

    document.documentElement.style.overflow = 'hidden'
    document.body.style.overflow = 'hidden'
    if (scrollbarWidth > 0) {
      document.body.style.paddingRight = `${scrollbarWidth}px`
    }

    scrollLocked = true
  }

  const unlockScroll = (): void => {
    if (!scrollLocked || typeof document === 'undefined') {
      return
    }

    document.documentElement.style.overflow = originalDocumentOverflow
    document.body.style.overflow = originalOverflow
    document.body.style.paddingRight = originalPaddingRight
    scrollLocked = false
  }

  const handleClose = (): void => {
    dispatch('close')
  }

  onDestroy(() => {
    unlockScroll()
  })

  $: if (open) {
    lockScroll()
  } else {
    unlockScroll()
  }
</script>

{#if open}
  <div use:portal>
    <ModalBackdrop on:close={handleClose} />

    <div class={`fixed inset-0 z-50 flex overscroll-contain p-3 sm:p-4 pointer-events-auto ${overlayClass}`}>
      <div class={`ui-surface-card pointer-events-auto flex w-full max-h-[calc(100dvh-1.5rem)] flex-col overflow-hidden sm:max-h-[calc(100dvh-2rem)] ${cardClass}`}>
        {#if $$slots.header}
          <header class={`shrink-0 ${headerClass}`}>
            <slot name="header" />
          </header>
        {/if}

        <div class={`min-h-0 overflow-y-auto ${bodyClass}`}>
          <slot />
        </div>

        {#if $$slots.footer}
          <footer class={`shrink-0 ${footerClass}`}>
            <slot name="footer" />
          </footer>
        {/if}
      </div>
    </div>
  </div>
{/if}
