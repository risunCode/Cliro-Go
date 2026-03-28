<script lang="ts">
  import { createEventDispatcher } from 'svelte'

  const dispatch = createEventDispatcher<{ close: void }>()

  const handleBackdropClick = (): void => {
    dispatch('close')
  }
</script>

<button
  aria-label="Close modal"
  class="fixed inset-0 z-40 bg-black/45 backdrop-blur-sm"
  on:click={handleBackdropClick}
  type="button"
/>
