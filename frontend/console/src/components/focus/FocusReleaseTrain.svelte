<script lang="ts">
  // The release train (ADR §9 P5): pipelines finished since the latest v*
  // tag, grouped by repository, and the "Start release" gate that starts a
  // focus pipeline asking for the fixed release plan (lib/focusRelease).
  // The plan gate of that pipeline still applies.
  import { onMount } from 'svelte'
  import { t } from '../../i18n'
  import { createFocusPipeline, getReleaseTrain } from '../../lib/api'
  import { releaseItemLabel, releaseRequest } from '../../lib/focusRelease'
  import { shortCwdLabel } from '../../lib/sessionLabels'
  import type { ReleaseTrainGroup } from '../../lib/types'

  interface Props {
    onNavigate: (path: string) => void
  }

  let { onNavigate }: Props = $props()

  // The command the stale-tags hint names: content, not translated text.
  const fetchTagsCommand = 'git fetch --tags'

  let groups = $state<ReleaseTrainGroup[]>([])
  let loaded = $state(false)
  let error = $state('')
  // The repository whose "Start release" gate is open.
  let confirming = $state<string | null>(null)
  let starting = $state(false)
  let startError = $state('')

  async function refresh() {
    try {
      groups = (await getReleaseTrain()).groups
      error = ''
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      loaded = true
    }
  }

  onMount(() => {
    void refresh()
  })

  async function start(group: ReleaseTrainGroup) {
    if (starting) return
    starting = true
    startError = ''
    try {
      const created = await createFocusPipeline(releaseRequest(group, $t.focus.release.sessionTitle(group.items.length)))
      onNavigate(`/console/focus/${encodeURIComponent(created.session_id)}`)
    } catch (err) {
      startError = $t.focus.release.startFailed(err instanceof Error ? err.message : String(err))
    } finally {
      starting = false
    }
  }

  function openSession(id: string) {
    onNavigate(`/console/focus/${encodeURIComponent(id)}`)
  }
</script>

<div class="release-train" data-testid="focus-release-train">
  <header class="release-header">
    <div>
      <h2>{$t.focus.release.title}</h2>
      <p class="subtitle">{$t.focus.release.subtitle}</p>
    </div>
    <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/focus')}>{$t.focus.release.back}</button>
  </header>

  {#if error}
    <p class="banner error" data-testid="focus-release-error">{$t.focus.release.loadFailed(error)}</p>
  {/if}

  {#if loaded && groups.length === 0 && !error}
    <p class="empty" data-testid="focus-release-empty">{$t.focus.release.empty}</p>
  {/if}

  {#each groups as group (group.repo)}
    <section class="repo-group" data-testid="focus-release-group">
      <header class="group-header">
        <span class="repo mono" title={group.repo} data-content>{shortCwdLabel(group.repo)}</span>
        <span class="badge badge-default">{group.last_tag ? $t.focus.release.since(group.last_tag) : $t.focus.release.untagged}</span>
        <span class="count mono">{$t.focus.release.count(group.items.length)}</span>
        {#if confirming !== group.repo}
          <button
            type="button"
            class="btn btn-primary btn-sm start"
            onclick={() => { confirming = group.repo; startError = '' }}
            data-testid="focus-release-start"
          >{$t.focus.release.start}</button>
        {/if}
      </header>

      {#if group.tags_stale}
        <p class="banner warning" data-testid="focus-release-stale">{$t.focus.release.staleTags(fetchTagsCommand)}</p>
      {/if}

      {#if confirming === group.repo}
        <div class="gate" role="group" aria-label={$t.focus.release.confirmTitle} data-testid="focus-release-gate">
          <strong>{$t.focus.release.confirmTitle}</strong>
          <p>{$t.focus.release.confirmBody}</p>
          {#if startError}<p class="banner error">{startError}</p>{/if}
          <div class="gate-actions">
            <button type="button" class="btn btn-ghost btn-sm" disabled={starting} onclick={() => { confirming = null }}>{$t.focus.release.cancel}</button>
            <button type="button" class="btn btn-primary btn-sm" disabled={starting} onclick={() => void start(group)} data-testid="focus-release-confirm">
              {starting ? $t.focus.release.starting : $t.focus.release.confirm}
            </button>
          </div>
        </div>
      {/if}

      <ul class="items">
        {#each group.items as item (item.session_id)}
          <li>
            <span class="item-main">
              {#if item.pr?.url}
                <a class="item-title" href={item.pr.url} target="_blank" rel="noreferrer" data-content>{releaseItemLabel(item)}</a>
              {:else}
                <button type="button" class="item-title link" onclick={() => openSession(item.session_id)} data-content>{releaseItemLabel(item)}</button>
              {/if}
              {#if item.goal && item.goal !== item.title}<span class="item-goal" data-content>{item.goal}</span>{/if}
            </span>
            <time class="mono when" datetime={item.finished_at}>{item.finished_at.slice(0, 10)}</time>
          </li>
        {/each}
      </ul>
    </section>
  {/each}
</div>

<style>
  .release-train {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 880px;
    margin: 0 auto;
    padding: var(--space-6) var(--space-6) var(--space-10);
  }

  .release-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  h2 {
    margin: 0;
  }

  .subtitle {
    margin: var(--space-1) 0 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .repo-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
  }

  .group-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .repo {
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .count {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .start {
    margin-left: auto;
  }

  .gate {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--primary);
    border-radius: var(--radius-md);
    background: var(--primary-muted);
  }

  .gate p {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .gate-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .items {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .items li {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    padding: var(--space-1) 0;
    border-top: 1px solid var(--border-subtle);
  }

  .item-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .item-title {
    color: var(--text-primary);
    font-size: var(--text-sm);
    text-align: left;
  }

  .item-title.link {
    padding: 0;
    border: none;
    background: none;
    cursor: pointer;
  }

  .item-title:hover {
    color: var(--primary-text);
  }

  .item-goal {
    font-size: var(--text-xs);
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .when {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .empty {
    margin: 0;
    padding: var(--space-8) var(--space-4);
    text-align: center;
    color: var(--text-tertiary);
    border: 1px dashed var(--border-default);
    border-radius: var(--radius-lg);
  }

  .banner.warning {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--warning-muted);
    color: var(--warning);
    font-size: var(--text-sm);
  }

  .banner.error {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--error-muted);
    color: var(--error);
    font-size: var(--text-sm);
  }
</style>
