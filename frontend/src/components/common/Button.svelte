<script lang="ts">
  import { cn } from '@/utils/cn'

  type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'
  type ButtonSize = 'sm' | 'md'

  export let variant: ButtonVariant = 'secondary'
  export let size: ButtonSize = 'md'
  export let className = ''
  export let type: 'button' | 'submit' | 'reset' = 'button'
  export let disabled = false

  const variantClasses: Record<ButtonVariant, string> = {
    primary: 'ui-btn-primary',
    secondary: 'ui-btn-secondary',
    ghost: 'ui-btn-ghost',
    danger: 'ui-btn-danger'
  }

  const sizeClasses: Record<ButtonSize, string> = {
    sm: 'px-3 py-2 text-xs',
    md: 'px-4 py-2'
  }
</script>

<button
  {...$$restProps}
  class={cn('ui-btn-base', variantClasses[variant], sizeClasses[size], className)}
  {disabled}
  {type}
  on:click
>
  <slot />
</button>
