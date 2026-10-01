<script lang="ts">
  // The /console/focus route: the focus home, or one session's pipeline
  // screen. Loaded lazily (lib/routeComponents.ts).
  import FocusHome from './FocusHome.svelte'
  import FocusPipeline from './FocusPipeline.svelte'

  interface Props {
    sessionId?: string
    onNavigate: (path: string) => void
  }

  let { sessionId, onNavigate }: Props = $props()
</script>

{#if sessionId}
  {#key sessionId}
    <FocusPipeline {sessionId} {onNavigate} />
  {/key}
{:else}
  <FocusHome {onNavigate} />
{/if}
