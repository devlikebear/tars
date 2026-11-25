<script lang="ts">
  // A finished project's saved versions (docs/plans/vibe-mode-redesign unit
  // 5, screens/D-Result): the newest three in a strip under the app, the
  // one the folder holds now marked, every other one with **Go back**; the
  // rest behind **All versions**. Going back asks first, then saves the old
  // files as a new version, so nothing in the list is ever lost. A refusal
  // (the team is at work: 409 busy) is told in plain words.
  import { onMount } from 'svelte'
  import { locale, t } from '../../i18n'
  import { easyErrorCode, localEasyApi, type EasyAPI } from '../../lib/api/easy'
  import { easyAgo } from '../../lib/easyBoard'
  import { VERSION_STRIP, currentVersion, restoreErrorKey, versionRows, type EasyVersion, type EasyVersionRow } from '../../lib/easy'

  let {
    projectId,
    onRestored,
    onCurrent,
    api = localEasyApi,
    // full shows every version at once (the phone's Versions tab) instead of
    // the strip of three.
    full = false,
  }: { projectId: string; onRestored: () => void; onCurrent?: (n: number | undefined) => void; api?: EasyAPI; full?: boolean } = $props()

  let versions = $state<EasyVersion[]>([])
  let loadFailed = $state(false)
  let restoring = $state(false)
  let restoreError = $state('')
  let all = $state(false)
  let asking = $state<EasyVersionRow | null>(null)
  let goButton = $state<HTMLButtonElement | null>(null)

  const rows = $derived(versionRows(versions, $t.easy.versions))
  const shown = $derived(all || full ? rows : rows.slice(0, VERSION_STRIP))

  $effect(() => {
    onCurrent?.(currentVersion(rows))
  })
  $effect(() => {
    if (asking) goButton?.focus()
  })

  onMount(async () => {
    try {
      versions = await api.versions(projectId)
    } catch {
      loadFailed = true
    }
  })

  function name(row: EasyVersionRow): string {
    return row.number ? $t.easy.versions.label(row.number) : row.label
  }

  async function restore() {
    const row = asking
    if (!row || restoring) return
    restoring = true
    restoreError = ''
    try {
      versions = await api.restore(projectId, row.commit)
      asking = null
      onRestored()
    } catch (err) {
      asking = null
      restoreError = $t.easy.versions.errors[restoreErrorKey(easyErrorCode(err))]
    } finally {
      restoring = false
    }
  }

  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && asking && !restoring) asking = null
  }
</script>

<svelte:window onkeydown={keydown} />

<section class="versions" data-testid="easy-versions">
  <h2 class="sr-only">{$t.easy.versions.title}</h2>
  {#if loadFailed}
    <p class="error">{$t.easy.versions.loadFailed}</p>
  {:else}
    <ul class="strip">
      {#each shown as row (row.commit)}
        <li class:current={row.current} data-testid="easy-version" data-current={row.current ? 'true' : 'false'}>
          <span class="what">
            <b>
              {name(row)}
              {#if row.current}<span class="now">{$t.easy.versions.current}</span>{/if}
            </b>
            <span class="line">{row.number ? `${row.label} · ` : ''}{easyAgo(row.at, $locale)}</span>
          </span>
          {#if !row.current}
            <button type="button" class="btn btn-secondary btn-sm" data-testid="easy-version-restore" disabled={restoring} onclick={() => (asking = row)}>
              {$t.easy.versions.restore}
            </button>
          {/if}
        </li>
      {/each}
    </ul>
    {#if !full && rows.length > VERSION_STRIP}
      <button type="button" class="more" data-testid="easy-versions-all" onclick={() => (all = !all)}>
        {all ? $t.easy.versions.fewer : $t.easy.versions.all(rows.length)}
      </button>
    {/if}
    {#if restoreError}<p class="error" role="alert" data-testid="easy-version-error">{restoreError}</p>{/if}
  {/if}
</section>

{#if asking}
  <div class="backdrop" role="presentation" onclick={() => !restoring && (asking = null)}></div>
  <div class="dialog" role="alertdialog" aria-modal="true" aria-labelledby="easy-restore-title" aria-describedby="easy-restore-body" data-testid="easy-restore-confirm">
    <h3 id="easy-restore-title">{$t.easy.versions.confirm.title(name(asking))}</h3>
    <p id="easy-restore-body">{$t.easy.versions.confirm.body}</p>
    <div class="dialog-actions">
      <button type="button" class="btn btn-ghost" disabled={restoring} onclick={() => (asking = null)}>{$t.easy.versions.confirm.cancel}</button>
      <button type="button" class="btn btn-primary" data-testid="easy-restore-go" bind:this={goButton} disabled={restoring} onclick={restore}>
        {restoring ? $t.easy.versions.restoring : $t.easy.versions.confirm.go}
      </button>
    </div>
  </div>
{/if}

<style>
  .versions {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
  }
  .error {
    color: var(--error);
    margin: 0;
  }
  .strip {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-3);
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface);
  }
  li.current {
    border-color: var(--primary);
  }
  .what {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
    font-size: var(--text-sm);
  }
  b {
    font-weight: 600;
  }
  .line {
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .now {
    margin-left: var(--space-1);
    font-weight: 400;
    font-size: var(--text-xs);
    padding: 1px var(--space-2);
    border-radius: 999px;
    background: var(--primary-muted);
    color: var(--primary-text);
  }
  .more {
    align-self: flex-start;
    font: inherit;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    background: transparent;
    border: 0;
    padding: var(--space-1) 0;
    cursor: pointer;
    text-decoration: underline;
  }
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    z-index: 40;
  }
  .dialog {
    position: fixed;
    z-index: 41;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    width: min(440px, calc(100vw - 32px));
    box-sizing: border-box;
    padding: var(--space-5);
    background: var(--surface-elevated);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: 0 16px 48px rgba(0, 0, 0, 0.5);
  }
  h3 {
    margin: 0 0 var(--space-2);
    font-size: var(--text-lg);
  }
  .dialog p {
    margin: 0 0 var(--space-4);
    color: var(--text-secondary);
    line-height: 1.6;
  }
  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
  @media (max-width: 900px) {
    .strip {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
