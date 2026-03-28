<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { Moon, Palette, Sun } from 'lucide-svelte'
  import type { Theme } from '@/stores/theme'
  import Button from '@/components/common/Button.svelte'
  import appIcon from '@/assets/icons/cliro-icon.png'
  import { APP_TABS, type AppTabId } from '@/utils/tabs'

  export let activeTab: AppTabId = 'dashboard'
  export let theme: Theme = 'light'

  const appVersion = 'v0.1.0'

  const dispatch = createEventDispatcher<{ tabChange: AppTabId; toggleTheme: void }>()

  const selectTab = (tab: AppTabId): void => {
    dispatch('tabChange', tab)
  }

  const onToggleTheme = (): void => {
    dispatch('toggleTheme')
  }

  const themeCycle: Theme[] = ['light', 'dark', 'solarized']

  const getThemeIcon = (value: Theme) => {
    if (value === 'light') {
      return Sun
    }

    if (value === 'dark') {
      return Moon
    }

    return Palette
  }

  const getNextThemeLabel = (value: Theme): string => {
    const currentIndex = themeCycle.indexOf(value)
    const nextTheme = themeCycle[(currentIndex + 1) % themeCycle.length]
    return nextTheme[0].toUpperCase() + nextTheme.slice(1)
  }
</script>

<header class="rounded-b-base border border-border bg-surface px-4 py-3 shadow-soft md:px-6">
  <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
    <div class="flex items-center gap-3">
      <div class="flex h-10 w-10 items-center justify-center rounded-sm border border-border bg-app text-text-primary">
        <img src={appIcon} alt="CLIro icon" class="h-6 w-6 rounded-[4px] object-cover" />
      </div>
      <div>
        <div class="flex items-center gap-2">
          <p class="text-sm font-semibold text-text-primary">CLIro</p>
          <span class="inline-flex items-center rounded-full border border-border bg-app px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-text-secondary">
            {appVersion}
          </span>
        </div>
        <p class="text-xs text-text-secondary">Operations Workspace</p>
      </div>
    </div>

    <div class="ml-auto flex items-center gap-2">
      <nav
        aria-label="Primary tabs"
        class="no-scrollbar flex max-w-[70vw] items-center gap-1 overflow-x-auto rounded-sm border border-border bg-app p-1"
      >
        {#each APP_TABS as tab}
          <Button
            aria-current={activeTab === tab.id ? 'page' : undefined}
            className={`px-3 py-1.5 !text-sm !font-medium ${
              activeTab === tab.id
                ? 'border-border bg-surface text-text-primary shadow-soft'
                : 'border-transparent text-text-secondary hover:text-text-primary'
            }`}
            on:click={() => selectTab(tab.id)}
            variant="ghost"
            size="sm"
          >
            {tab.label}
          </Button>
        {/each}
      </nav>

      <Button
        aria-label={`Switch theme to ${getNextThemeLabel(theme)}`}
        className="h-9 justify-center gap-1.5 px-2 text-text-primary"
        on:click={onToggleTheme}
        size="sm"
        variant="secondary"
      >
        <svelte:component this={getThemeIcon(theme)} size={16} strokeWidth={2} />
        <span class="text-[10px] font-semibold uppercase tracking-[0.06em]">{theme}</span>
      </Button>
    </div>
  </div>
</header>
