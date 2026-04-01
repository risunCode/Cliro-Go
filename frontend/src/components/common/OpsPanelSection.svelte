<script lang="ts">
  import { cn } from '@/shared/lib/cn'

  type AccentTone = 'amber' | 'cyan' | 'emerald' | 'rose' | 'slate'

  export let kicker = ''
  export let title = ''
  export let description = ''
  export let icon: any = null
  export let accent: AccentTone = 'slate'
  export let className = ''
  export let bodyClassName = ''

  const accentClasses: Record<AccentTone, string> = {
    amber: 'ops-panel-accent-amber',
    cyan: 'ops-panel-accent-cyan',
    emerald: 'ops-panel-accent-emerald',
    rose: 'ops-panel-accent-rose',
    slate: 'ops-panel-accent-slate'
  }
</script>

<section class={cn('ops-panel-section', accentClasses[accent], className)}>
  <div class="ops-panel-head">
    <div class="ops-panel-heading">
      {#if icon}
        <span class="ops-panel-icon-wrap">
          <svelte:component this={icon} size={16} />
        </span>
      {/if}

      <div class="ops-panel-copy">
        {#if kicker}
          <p class="ops-panel-kicker">{kicker}</p>
        {/if}
        {#if title}
          <h4 class="ops-panel-title">{title}</h4>
        {/if}
        {#if description}
          <p class="ops-panel-description">{description}</p>
        {/if}
      </div>
    </div>

    {#if $$slots.aside}
      <div class="ops-panel-aside">
        <slot name="aside" />
      </div>
    {/if}
  </div>

  <div class={cn('ops-panel-body', bodyClassName)}>
    <slot />
  </div>
</section>

<style>
  .ops-panel-section {
    --ops-panel-accent: #64748b;

    position: relative;
    border: 1px solid color-mix(in srgb, var(--color-border) 82%, white 8%);
    border-radius: 14px;
    background:
      linear-gradient(180deg, color-mix(in srgb, var(--color-surface) 95%, white 2%) 0%, color-mix(in srgb, var(--color-surface) 86%, var(--color-app) 14%) 100%);
    box-shadow: inset 0 1px 0 color-mix(in srgb, white 5%, transparent);
    overflow: hidden;
  }

  .ops-panel-section::before {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    top: 0;
    height: 2px;
    background: linear-gradient(90deg, color-mix(in srgb, var(--ops-panel-accent) 70%, transparent), transparent 72%);
    opacity: 0.9;
  }

  .ops-panel-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    padding: 1rem 1rem 0.95rem;
    border-bottom: 1px solid color-mix(in srgb, var(--color-border) 90%, transparent);
  }

  .ops-panel-heading {
    min-width: 0;
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
  }

  .ops-panel-icon-wrap {
    width: 2rem;
    height: 2rem;
    border-radius: 0.75rem;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 1px solid color-mix(in srgb, var(--ops-panel-accent) 20%, var(--color-border));
    background: color-mix(in srgb, var(--ops-panel-accent) 10%, var(--color-app));
    color: color-mix(in srgb, var(--ops-panel-accent) 66%, var(--color-text-primary));
    flex-shrink: 0;
  }

  .ops-panel-copy {
    min-width: 0;
  }

  .ops-panel-kicker {
    margin-bottom: 0.25rem;
    font-size: 0.62rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--color-text-secondary);
  }

  .ops-panel-title {
    font-size: 0.98rem;
    line-height: 1.2;
    font-weight: 700;
    letter-spacing: -0.015em;
    color: var(--color-text-primary);
  }

  .ops-panel-description {
    margin-top: 0.35rem;
    font-size: 0.75rem;
    line-height: 1.45;
    color: var(--color-text-secondary);
  }

  .ops-panel-aside {
    flex-shrink: 0;
  }

  .ops-panel-body {
    padding: 1rem;
  }

  .ops-panel-accent-amber {
    --ops-panel-accent: #f59e0b;
  }

  .ops-panel-accent-cyan {
    --ops-panel-accent: #06b6d4;
  }

  .ops-panel-accent-emerald {
    --ops-panel-accent: #10b981;
  }

  .ops-panel-accent-rose {
    --ops-panel-accent: #f43f5e;
  }

  .ops-panel-accent-slate {
    --ops-panel-accent: #64748b;
  }

  @media (max-width: 767px) {
    .ops-panel-head {
      flex-direction: column;
      align-items: stretch;
    }
  }
</style>
