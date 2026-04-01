<script lang="ts">
  import { Check, Copy } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'

  export let label = ''
  export let value = ''
  export let displayValue = '-'
  export let copied = false
  export let canCopy = false
  export let onCopy: () => Promise<void> | void
</script>

<div class="rounded-sm border border-border bg-surface p-2">
  <div class="mb-1 flex items-center justify-between gap-2">
    <p class="text-[10px] uppercase tracking-[0.06em] text-text-secondary">{label}</p>
    <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-[11px]" type="button" disabled={!value || !canCopy} on:click={onCopy}>
      {#if copied}
        <Check size={12} />
        Copied
      {:else}
        <Copy size={12} />
        Copy
      {/if}
    </Button>
  </div>
  <p class="break-all font-mono text-[11px] text-text-primary">{displayValue || '-'}</p>
</div>
