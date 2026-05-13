<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { t } from '../i18n'
  import type { HubDryRunResult } from '../lib/types'

  let { preview, oncancel, onconfirm }: {
    preview: HubDryRunResult
    oncancel: () => void
    onconfirm: () => void | Promise<void>
  } = $props()

  let installing = $state(false)
  let attributionOpen = $state(false)

  function shortHash(h: string): string {
    if (!h) return ''
    return h.length <= 12 ? h : h.slice(0, 12)
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  async function handleConfirm() {
    if (installing) return
    installing = true
    try {
      await onconfirm()
    } finally {
      installing = false
    }
  }

  function handleKey(ev: KeyboardEvent) {
    if (ev.key === 'Escape' && !installing) {
      oncancel()
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKey)
  })
  onDestroy(() => {
    window.removeEventListener('keydown', handleKey)
  })
</script>

<div class="dryrun-backdrop" role="dialog" aria-modal="true" aria-labelledby="dryrun-title">
  <div class="dryrun-modal">
    <header class="dryrun-header">
      <h2 id="dryrun-title" class="dryrun-title">{$t.extensions.dryRunTitle}</h2>
      <button class="dryrun-close" aria-label="close" onclick={oncancel} disabled={installing}>×</button>
    </header>

    <div class="dryrun-body">
      <dl class="dryrun-meta">
        <dt>{$t.extensions.dryRunSource}</dt>
        <dd><span class="ext-source-badge ext-source-badge-external">{preview.source_id}</span></dd>
        <dt>{$t.extensions.dryRunSkill}</dt>
        <dd>{preview.original_name}</dd>
        {#if preview.original_url}
          <dt>{$t.extensions.dryRunOriginURL}</dt>
          <dd><a href={preview.original_url} target="_blank" rel="noopener">{preview.original_url}</a></dd>
        {:else if preview.original_path}
          <dt>{$t.extensions.dryRunOriginPath}</dt>
          <dd>{preview.original_path}</dd>
        {/if}
        <dt>{$t.extensions.dryRunTargetDir}</dt>
        <dd class="dryrun-mono">{preview.target_dir}</dd>
        {#if preview.license_label}
          <dt>{$t.extensions.dryRunLicense}</dt>
          <dd>{preview.license_label}</dd>
        {/if}
      </dl>

      {#if preview.converted_skill}
        <section class="dryrun-section">
          <h3>{$t.extensions.dryRunConvertedFrontmatter}</h3>
          <dl class="dryrun-frontmatter">
            <dt>name</dt><dd>{preview.converted_skill.name}</dd>
            <dt>description</dt><dd>{preview.converted_skill.description}</dd>
            {#if preview.converted_skill.version}
              <dt>version</dt><dd>{preview.converted_skill.version}</dd>
            {/if}
            {#if preview.converted_skill.author}
              <dt>author</dt><dd>{preview.converted_skill.author}</dd>
            {/if}
            {#if preview.converted_skill.tags?.length}
              <dt>tags</dt><dd>[{preview.converted_skill.tags.join(', ')}]</dd>
            {/if}
          </dl>
        </section>
      {/if}

      <section class="dryrun-section">
        <h3>{$t.extensions.dryRunFiles(preview.files?.length ?? 0)}</h3>
        <ul class="dryrun-files">
          {#each preview.files ?? [] as fp}
            <li class="dryrun-file" class:mismatch={fp.expected_sha256 && fp.expected_sha256.toLowerCase() !== fp.sha256.toLowerCase()}>
              <span class="dryrun-file-path">{fp.path}</span>
              <span class="dryrun-file-size">{formatSize(fp.size)}</span>
              <span class="dryrun-file-hash" title={fp.sha256}>sha256:{shortHash(fp.sha256)}</span>
              {#if fp.expected_sha256 && fp.expected_sha256.toLowerCase() !== fp.sha256.toLowerCase()}
                <span class="dryrun-file-warning">{$t.extensions.dryRunChecksumMismatch}</span>
              {/if}
            </li>
          {/each}
        </ul>
      </section>

      {#if preview.adapter_warnings?.length}
        <section class="dryrun-section">
          <h3 class="dryrun-warning-heading">{$t.extensions.dryRunAdapterWarnings}</h3>
          <ul class="dryrun-warnings">
            {#each preview.adapter_warnings as msg}
              <li>{msg}</li>
            {/each}
          </ul>
        </section>
      {/if}

      {#if preview.checksum_warnings?.length}
        <section class="dryrun-section">
          <h3 class="dryrun-warning-heading">{$t.extensions.dryRunChecksumWarnings}</h3>
          <ul class="dryrun-warnings">
            {#each preview.checksum_warnings as msg}
              <li>{msg}</li>
            {/each}
          </ul>
        </section>
      {/if}

      {#if preview.license_source}
        <section class="dryrun-section">
          <details bind:open={attributionOpen}>
            <summary>{$t.extensions.dryRunAttribution(preview.license_label ?? '')}</summary>
            <p class="dryrun-attribution-note">{$t.extensions.dryRunAttributionNote}</p>
          </details>
        </section>
      {/if}
    </div>

    <footer class="dryrun-footer">
      <button class="btn btn-default" onclick={oncancel} disabled={installing}>
        {$t.extensions.dryRunCancel}
      </button>
      <button class="btn btn-primary" onclick={handleConfirm} disabled={installing}>
        {installing ? $t.extensions.installing : $t.extensions.dryRunConfirm}
      </button>
    </footer>
  </div>
</div>

<style>
  .dryrun-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: var(--space-4);
  }

  .dryrun-modal {
    background: var(--surface-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    max-width: 720px;
    width: 100%;
    max-height: 90vh;
    display: flex;
    flex-direction: column;
  }

  .dryrun-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
  }
  .dryrun-title {
    margin: 0;
    font-size: var(--text-sm);
    font-family: var(--font-display);
    font-weight: 500;
    color: var(--text-primary);
  }
  .dryrun-close {
    background: transparent;
    border: none;
    color: var(--text-secondary);
    font-size: 20px;
    cursor: pointer;
    padding: 0 var(--space-2);
  }
  .dryrun-close:hover:not(:disabled) {
    color: var(--text-primary);
  }

  .dryrun-body {
    flex: 1;
    overflow-y: auto;
    padding: var(--space-4);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .dryrun-meta,
  .dryrun-frontmatter {
    display: grid;
    grid-template-columns: 140px 1fr;
    gap: 4px var(--space-3);
    margin: 0;
    font-size: var(--text-xs);
  }
  .dryrun-meta dt,
  .dryrun-frontmatter dt {
    color: var(--text-tertiary);
    font-family: var(--font-display);
  }
  .dryrun-meta dd,
  .dryrun-frontmatter dd {
    margin: 0;
    color: var(--text-primary);
    font-family: var(--font-mono);
    word-break: break-all;
  }
  .dryrun-mono { font-family: var(--font-mono); }

  .dryrun-section h3 {
    margin: 0 0 var(--space-2) 0;
    font-size: var(--text-xs);
    font-family: var(--font-display);
    font-weight: 500;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  .dryrun-warning-heading { color: #e09145; }

  .dryrun-files {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .dryrun-file {
    display: grid;
    grid-template-columns: 1fr auto auto;
    gap: var(--space-3);
    padding: 2px 0;
    font-size: var(--text-xs);
    font-family: var(--font-mono);
    color: var(--text-primary);
  }
  .dryrun-file.mismatch { color: #e09145; }
  .dryrun-file-size,
  .dryrun-file-hash {
    color: var(--text-tertiary);
  }
  .dryrun-file-warning {
    grid-column: 1 / -1;
    color: #e09145;
    font-size: 10px;
    padding-left: var(--space-3);
  }

  .dryrun-warnings {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: var(--text-xs);
    font-family: var(--font-mono);
    color: #e09145;
  }
  .dryrun-warnings li::before { content: '⚠ '; }

  .dryrun-attribution-note {
    margin: var(--space-2) 0 0 0;
    font-size: var(--text-xs);
    color: var(--text-secondary);
    font-family: var(--font-mono);
  }

  .dryrun-footer {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--border-subtle);
  }

  .ext-source-badge {
    display: inline-flex;
    align-items: center;
    padding: 1px 6px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-subtle);
    background: rgba(255, 255, 255, 0.04);
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: 10px;
    letter-spacing: 0.5px;
  }
  .ext-source-badge-external {
    border-color: rgba(224, 145, 69, 0.4);
    color: #e09145;
    background: rgba(224, 145, 69, 0.08);
  }
</style>
