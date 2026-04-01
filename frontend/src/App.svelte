<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import AppOverlayStack from '@/app/providers/AppOverlayStack.svelte'
  import { createAppController } from '@/app/services/app-controller'
  import AppFrame from '@/app/shell/AppFrame.svelte'
  import { theme, toggleTheme } from '@/shared/stores/theme'

  const controller = createAppController()
  const { shell, overlays, appActions, accountsActions, routerActions, logsActions, settingsActions } = controller

  onMount(() => {
    void controller.initialize()
  })

  onDestroy(() => {
    controller.destroy()
  })
</script>

<main class="h-screen overflow-hidden bg-app text-text-primary">
  <AppFrame
    shell={$shell}
    theme={$theme}
    {appActions}
    {accountsActions}
    {routerActions}
    {logsActions}
    {settingsActions}
    onToggleTheme={toggleTheme}
  />

  <AppOverlayStack overlays={$overlays} {appActions} {settingsActions} />
</main>
