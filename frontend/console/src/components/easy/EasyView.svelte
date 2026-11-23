<script lang="ts">
  // Easy mode's route: setting TARS up on its first run, the make screen,
  // or one project's progress.
  import EasyHome from './EasyHome.svelte'
  import EasyProject from './EasyProject.svelte'
  import EasySetup from './EasySetup.svelte'

  let {
    projectId,
    needsSetup = false,
    onNavigate,
  }: { projectId?: string; needsSetup?: boolean; onNavigate: (path: string) => void } = $props()
</script>

{#if needsSetup}
  <EasySetup {onNavigate} />
{:else if projectId}
  {#key projectId}
    <EasyProject {projectId} {onNavigate} />
  {/key}
{:else}
  <EasyHome {onNavigate} />
{/if}
