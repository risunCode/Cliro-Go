<script lang="ts">
  import { cn } from '@/utils/cn'

  export let as: 'article' | 'div' | 'section' = 'article'
  export let className = ''
</script>

<svelte:element this={as} {...$$restProps} class={cn('ui-surface-card', className)}>
  <slot />
</svelte:element>
