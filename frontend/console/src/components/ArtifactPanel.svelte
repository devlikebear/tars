<script lang="ts">
  import type { Artifact } from '../lib/artifacts'
  import { fileIcon } from '../lib/artifacts'

  interface Props {
    artifacts: Artifact[]
    onClose: () => void
  }

  let { artifacts, onClose }: Props = $props()

  function relativeTime(ts: number): string {
    const seconds = Math.floor((Date.now() - ts) / 1000)
    if (seconds < 60) return `${seconds}s ago`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
    return `${Math.floor(seconds / 3600)}h ago`
  }

  function basename(path: string): string {
    return path.split('/').pop() || path
  }

  function dirname(path: string): string {
    const parts = path.split('/')
    if (parts.length <= 1) return ''
    return parts.slice(0, -1).join('/')
  }
</script>

<aside class="artifact-panel">
  <div class="artifact-header">
    <span class="artifact-title">Artifacts</span>
    <span class="artifact-count">{artifacts.length}</span>
    <button type="button" class="artifact-close" onclick={onClose}>&times;</button>
  </div>

  <div class="artifact-list">
    {#if artifacts.length === 0}
      <div class="artifact-empty">No files created yet.</div>
    {:else}
      {#each artifacts as artifact}
        <div class="artifact-item">
          <span class="artifact-icon">{fileIcon(artifact.path)}</span>
          <div class="artifact-info">
            <span class="artifact-name">{basename(artifact.path)}</span>
            {#if dirname(artifact.path)}
              <span class="artifact-dir">{dirname(artifact.path)}</span>
            {/if}
          </div>
          <div class="artifact-meta">
            <span class="badge {artifact.action === 'created' ? 'badge-success' : 'badge-accent'}" style="font-size:9px;padding:1px 5px">
              {artifact.action}
            </span>
            <span class="artifact-time">{relativeTime(artifact.timestamp)}</span>
          </div>
        </div>
      {/each}
    {/if}
  </div>
</aside>

<style>
  .artifact-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    width: 280px;
  }

  .artifact-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .artifact-title {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
  }

  .artifact-count {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-ghost);
    background: var(--bg-elevated);
    padding: 1px 6px;
    border-radius: var(--radius-sm);
  }

  .artifact-close {
    margin-left: auto;
    background: none;
    border: none;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: var(--text-md);
    padding: 0 2px;
    line-height: 1;
  }
  .artifact-close:hover { color: var(--text-primary); }

  .artifact-list {
    flex: 1;
    overflow-y: auto;
    padding: var(--space-2);
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .artifact-empty {
    padding: var(--space-4);
    text-align: center;
    color: var(--text-ghost);
    font-size: var(--text-xs);
  }

  .artifact-item {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    transition: background var(--duration-fast) var(--ease-out);
  }
  .artifact-item:hover {
    background: var(--bg-hover);
  }

  .artifact-icon {
    font-size: var(--text-md);
    flex-shrink: 0;
    width: 20px;
    text-align: center;
  }

  .artifact-info {
    flex: 1;
    min-width: 0;
  }

  .artifact-name {
    display: block;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .artifact-dir {
    display: block;
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .artifact-meta {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 2px;
    flex-shrink: 0;
  }

  .artifact-time {
    font-size: 10px;
    color: var(--text-ghost);
  }
</style>
