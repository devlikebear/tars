<script lang="ts">
  // The /console/focus route: the focus home, or one session's pipeline
  // screen, or the release train. Loaded lazily (lib/routeComponents.ts).
  import FocusHome from './FocusHome.svelte'
  import FocusPipeline from './FocusPipeline.svelte'
  import FocusReleaseTrain from './FocusReleaseTrain.svelte'

  interface Props {
    sessionId?: string
    release?: boolean
    onNavigate: (path: string) => void
  }

  let { sessionId, release = false, onNavigate }: Props = $props()
</script>

{#if release}
  <FocusReleaseTrain {onNavigate} />
{:else if sessionId}
  {#key sessionId}
    <FocusPipeline {sessionId} {onNavigate} />
  {/key}
{:else}
  <FocusHome {onNavigate} />
{/if}
