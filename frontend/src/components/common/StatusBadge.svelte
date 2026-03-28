<script lang="ts">
  import { cn } from '@/utils/cn'

  type BadgeTone = 'neutral' | 'success' | 'error' | 'info' | 'warning'

  export let tone: BadgeTone = 'neutral'
  export let className = ''

  const toneClasses: Record<BadgeTone, string> = {
    neutral: 'ui-status-badge-neutral',
    success: 'ui-status-badge-success',
    error: 'ui-status-badge-error',
    info: 'ui-status-badge-info',
    warning: 'ui-status-badge-warning'
  }
</script>

<span class={cn('ui-status-badge', toneClasses[tone], className)}>
  <slot />
</span>
