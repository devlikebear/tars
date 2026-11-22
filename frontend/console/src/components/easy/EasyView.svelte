<script lang="ts">
  // Easy mode's route: the make screen, or one project's progress.
  import EasyHome from './EasyHome.svelte'
  import EasyProject from './EasyProject.svelte'

  let { projectId, onNavigate }: { projectId?: string; onNavigate: (path: string) => void } = $props()
</script>

{#if projectId}
  {#key projectId}
    <EasyProject {projectId} {onNavigate} />
  {/key}
{:else}
  <EasyHome {onNavigate} />
{/if}
