<script lang="ts">
  // Easy mode's route: setting TARS up on its first run, or, inside the
  // vibe shell, the make screen (home), the project board and its archive,
  // or one project's progress.
  import EasyBoard from './EasyBoard.svelte'
  import EasyHome from './EasyHome.svelte'
  import EasyProject from './EasyProject.svelte'
  import EasySetup from './EasySetup.svelte'
  import EasyShell from './EasyShell.svelte'

  let {
    projectId,
    page,
    needsSetup = false,
    onNavigate,
  }: { projectId?: string; page?: 'projects' | 'archive'; needsSetup?: boolean; onNavigate: (path: string) => void } = $props()

  // A screen below changed a project: the shell reads its count again.
  let tick = $state(0)
  const changed = () => tick++
</script>

{#if needsSetup}
  <EasySetup {onNavigate} />
{:else}
  <EasyShell active={projectId ? 'project' : page ? 'projects' : 'make'} {tick} {onNavigate}>
    {#if projectId}
      {#key projectId}
        <EasyProject {projectId} {onNavigate} />
      {/key}
    {:else if page}
      {#key page}
        <EasyBoard archive={page === 'archive'} {onNavigate} onChanged={changed} />
      {/key}
    {:else}
      <EasyHome {onNavigate} />
    {/if}
  </EasyShell>
{/if}
