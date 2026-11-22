<script lang="ts">
  // A finished project's saved versions (docs/decisions/vibe-coder-identity.md
  // §4, phase 3): newest first, the one the folder holds now marked, every
  // other one with "go back to this". Going back saves the old files as a new
  // version, so nothing in the list is ever lost.
  import { onMount } from 'svelte'
  import { locale, t } from '../../i18n'
  import { listEasyVersions, restoreEasyVersion } from '../../lib/api/easy'
  import { versionLabel, type EasyVersion } from '../../lib/easy'

  let { projectId, onRestored }: { projectId: string; onRestored: () => void } = $props()

  let versions = $state<EasyVersion[]>([])
  let loadFailed = $state(false)
  let restoring = $state('')
  let restoreFailed = $state(false)

  onMount(async () => {
    try {
      versions = await listEasyVersions(projectId)
    } catch {
      loadFailed = true
    }
  })

  async function restore(commit: string) {
    if (restoring) return
    restoring = commit
    restoreFailed = false
    try {
      versions = await restoreEasyVersion(projectId, commit)
      onRestored()
    } catch {
      restoreFailed = true
    } finally {
      restoring = ''
    }
  }

  function when(at: string): string {
    const d = new Date(at)
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString($locale, { dateStyle: 'medium', timeStyle: 'short' })
  }
</script>

<section class="versions" data-testid="easy-versions">
  <h2>{$t.easy.versions.title}</h2>
  <p class="muted">{$t.easy.versions.hint}</p>
  {#if loadFailed}
    <p class="error">{$t.easy.versions.loadFailed}</p>
  {:else}
    <ul>
      {#each versions as v (v.commit)}
        <li class:current={v.current} data-testid="easy-version" data-current={v.current ? 'true' : 'false'}>
          <span class="what">
            <span class="title">{versionLabel(v, $t.easy.versions)}</span>
            <span class="when">{when(v.at)}</span>
          </span>
          {#if v.current}
            <span class="now">{$t.easy.versions.current}</span>
          {:else}
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              data-testid="easy-version-restore"
              disabled={Boolean(restoring)}
              onclick={() => restore(v.commit)}
            >
              {restoring === v.commit ? $t.easy.versions.restoring : $t.easy.versions.restore}
            </button>
          {/if}
        </li>
      {/each}
    </ul>
    {#if restoreFailed}<p class="error" role="alert">{$t.easy.versions.failed}</p>{/if}
  {/if}
</section>

<style>
  h2 {
    font-size: var(--text-lg);
    margin: 0 0 var(--space-2);
  }
  .muted {
    color: var(--text-secondary);
    margin: 0 0 var(--space-3);
  }
  .error {
    color: var(--error);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
  }
  li.current {
    border-color: rgba(var(--primary-rgb), 0.45);
  }
  .what {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
  }
  .title {
    overflow-wrap: anywhere;
  }
  .when {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }
  .now {
    font-size: var(--text-xs);
    padding: 2px var(--space-2);
    border-radius: 999px;
    background: var(--success-muted);
    color: var(--success);
  }
</style>
