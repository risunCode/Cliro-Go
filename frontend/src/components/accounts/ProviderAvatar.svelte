<script lang="ts">
  import codexIcon from '@/assets/icons/codex-icon.png'
  import kiroIcon from '@/assets/icons/kiro-icon.png'
  import { normalizeProviderID, providerMeta } from '@/utils/account'

  export let provider = ''
  export let variant: 'icon' | 'chip' = 'icon'
  export let showLabel = false

  $: providerID = normalizeProviderID(provider)
  $: meta = providerMeta(providerID)
  $: iconSrc = providerID === 'codex' ? codexIcon : providerID === 'kiro' ? kiroIcon : ''
  $: hasProviderIcon = iconSrc.length > 0
</script>

{#if variant === 'chip'}
  <span class="provider-chip-dot" style={hasProviderIcon ? undefined : `background:${meta.tint}`}>
    {#if hasProviderIcon}
      <img src={iconSrc} alt={meta.label} class="provider-chip-image" loading="lazy" decoding="async" />
    {:else}
      {meta.marker}
    {/if}
  </span>
{:else}
  <div class="provider-icon" style={hasProviderIcon ? undefined : `background:${meta.tint}`}>
    {#if hasProviderIcon}
      <img src={iconSrc} alt={meta.label} class="provider-icon-image" loading="lazy" decoding="async" />
    {:else}
      {meta.marker}
    {/if}
  </div>
{/if}

{#if showLabel}
  <span>{meta.label}</span>
{/if}
