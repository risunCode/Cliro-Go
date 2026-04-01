<script lang="ts">
  import { Link2 } from 'lucide-svelte'
  import codexIcon from '@/assets/icons/codex-icon.png'
  import kiroIcon from '@/assets/icons/kiro-icon.png'
  import Button from '@/components/common/Button.svelte'
  import CollapsibleSurfaceSection from '@/components/common/CollapsibleSurfaceSection.svelte'

  export let open = false
  export let authWorking = false
  export let onSelectProvider: (provider: 'codex' | 'kiro') => Promise<void>
</script>

<CollapsibleSurfaceSection
  bind:open
  icon={Link2}
  title="Connect Accounts"
  subtitle="Connect Codex (OAuth) or Kiro (AWS Builder ID device flow)."
  pill={authWorking ? 'Connecting' : 'Auth'}
  ariaLabel="Toggle connect accounts section"
  className="accounts-connect-section p-0"
  bodyClassName="accounts-connect-body"
>
  <div class="accounts-connect-grid">
    <article class="accounts-connect-card">
      <div class="accounts-connect-card-head">
        <span class="accounts-connect-provider-icon">
          <img src={codexIcon} alt="Codex" loading="lazy" decoding="async" />
        </span>
        <div>
          <p class="accounts-connect-card-title">Codex (OpenAI)</p>
          <p class="accounts-connect-card-note">OAuth callback flow</p>
        </div>
      </div>
      <Button variant="primary" size="sm" disabled={authWorking} on:click={() => void onSelectProvider('codex')}>
        Connect Codex
      </Button>
    </article>

    <article class="accounts-connect-card">
      <div class="accounts-connect-card-head">
        <span class="accounts-connect-provider-icon">
          <img src={kiroIcon} alt="Kiro" loading="lazy" decoding="async" />
        </span>
        <div>
          <p class="accounts-connect-card-title">Kiro</p>
          <p class="accounts-connect-card-note">OAuth and AWS Builder ID device flow</p>
        </div>
      </div>
      <Button variant="secondary" size="sm" disabled={authWorking} on:click={() => void onSelectProvider('kiro')}>
        Connect Kiro
      </Button>
    </article>
  </div>
</CollapsibleSurfaceSection>
