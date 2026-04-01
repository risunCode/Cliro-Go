<script lang="ts">
  import type { AppActions, AppOverlayState, SettingsActions } from '@/app/services/app-controller'
  import ToastViewport from '@/components/common/ToastViewport.svelte'
  import ConfigurationRecoveryModal from '@/app/modals/ConfigurationRecoveryModal.svelte'
  import UpdateRequiredModal from '@/app/modals/UpdateRequiredModal.svelte'

  export let overlays: AppOverlayState
  export let appActions: AppActions
  export let settingsActions: SettingsActions
</script>

<ToastViewport />

<ConfigurationRecoveryModal
  open={overlays.showConfigurationErrorModal}
  warnings={overlays.startupWarnings}
  on:dismiss={appActions.dismissConfigurationErrorModal}
  on:openDataDir={settingsActions.openDataDir}
/>

<UpdateRequiredModal
  open={overlays.showUpdatePrompt}
  currentVersion={overlays.updateInfo?.currentVersion || ''}
  latestVersion={overlays.updateInfo?.latestVersion || ''}
  releaseName={overlays.updateInfo?.releaseName || ''}
  releaseUrl={overlays.updateInfo?.releaseUrl || ''}
  on:dismiss={appActions.dismissUpdatePrompt}
  on:openRelease={appActions.openUpdateReleasePage}
/>
