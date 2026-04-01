<script lang="ts">
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'
  import { cn } from '@/shared/lib/cn'

  type AccentTone = 'amber' | 'cyan' | 'emerald' | 'rose' | 'slate'

  export let kicker = ''
  export let title = ''
  export let subtitle = ''
  export let accent: AccentTone = 'amber'
  export let className = ''
  export let bodyClassName = ''

  const accentClasses: Record<AccentTone, string> = {
    amber: 'workspace-accent-amber',
    cyan: 'workspace-accent-cyan',
    emerald: 'workspace-accent-emerald',
    rose: 'workspace-accent-rose',
    slate: 'workspace-accent-slate'
  }
</script>

<SurfaceCard className={cn('control-workspace-card p-0 overflow-hidden', accentClasses[accent], className)}>
  <div class="workspace-shell">
    <div class="workspace-header">
      <div class="workspace-copy">
        {#if kicker}
          <p class="workspace-kicker">{kicker}</p>
        {/if}
        {#if title}
          <h3 class="workspace-title">{title}</h3>
        {/if}
        {#if subtitle}
          <p class="workspace-subtitle">{subtitle}</p>
        {/if}
      </div>

      {#if $$slots.headerAside}
        <div class="workspace-header-aside">
          <slot name="headerAside" />
        </div>
      {/if}
    </div>

    <div class={cn('workspace-body', bodyClassName)}>
      <slot />
    </div>
  </div>
</SurfaceCard>

<style>
  :global(.control-workspace-card) {
    --workspace-accent: #f59e0b;

    position: relative;
    border: 1px solid color-mix(in srgb, var(--color-border) 88%, white 6%);
    background: color-mix(in srgb, var(--color-surface) 94%, var(--color-app) 6%);
    box-shadow:
      inset 0 1px 0 color-mix(in srgb, white 5%, transparent),
      0 14px 28px rgba(0, 0, 0, 0.14);
  }

  :global(.workspace-accent-amber) {
    --workspace-accent: #f59e0b;
  }

  :global(.workspace-accent-cyan) {
    --workspace-accent: #06b6d4;
  }

  :global(.workspace-accent-emerald) {
    --workspace-accent: #10b981;
  }

  :global(.workspace-accent-rose) {
    --workspace-accent: #f43f5e;
  }

  :global(.workspace-accent-slate) {
    --workspace-accent: #64748b;
  }

  .workspace-shell {
    position: relative;
    padding: 1.2rem;
    background:
      linear-gradient(180deg, color-mix(in srgb, var(--workspace-accent) 7%, transparent) 0%, transparent 24%),
      linear-gradient(180deg, color-mix(in srgb, white 2%, transparent) 0%, transparent 100%),
      linear-gradient(180deg, color-mix(in srgb, var(--color-surface) 96%, white 2%) 0%, color-mix(in srgb, var(--color-surface) 88%, var(--color-app) 12%) 100%);
  }

  .workspace-shell::before {
    content: '';
    position: absolute;
    left: 1.2rem;
    right: 1.2rem;
    top: 0;
    height: 2px;
    border-radius: 999px;
    background: linear-gradient(90deg, color-mix(in srgb, var(--workspace-accent) 72%, transparent), transparent 72%);
    opacity: 0.85;
  }

  .workspace-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1rem;
    padding-bottom: 1rem;
    border-bottom: 1px solid color-mix(in srgb, var(--color-border) 88%, transparent);
  }

  .workspace-copy {
    min-width: 0;
  }

  .workspace-kicker {
    margin-bottom: 0.35rem;
    font-size: 0.62rem;
    font-weight: 700;
    letter-spacing: 0.11em;
    text-transform: uppercase;
    color: color-mix(in srgb, var(--workspace-accent) 58%, var(--color-text-secondary));
  }

  .workspace-title {
    font-size: 1.12rem;
    line-height: 1.2;
    font-weight: 700;
    letter-spacing: -0.02em;
    color: var(--color-text-primary);
  }

  .workspace-subtitle {
    margin-top: 0.42rem;
    max-width: 44rem;
    font-size: 0.77rem;
    line-height: 1.5;
    color: var(--color-text-secondary);
  }

  .workspace-header-aside {
    flex-shrink: 0;
  }

  .workspace-body {
    min-width: 0;
  }

  @media (max-width: 767px) {
    .workspace-header {
      flex-direction: column;
      align-items: stretch;
    }

    .workspace-header-aside {
      width: 100%;
    }
  }
</style>
