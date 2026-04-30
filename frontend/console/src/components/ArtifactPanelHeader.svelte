<script lang="ts">
  type Tab = 'session' | 'workspace'

  interface Props {
    activeTab: Tab
    artifactCount: number
    onSelectTab: (tab: Tab) => void
    onClose: () => void
  }

  let { activeTab, artifactCount, onSelectTab, onClose }: Props = $props()
</script>

<div class="artifact-header">
  <span class="artifact-title">Files</span>
  <div class="artifact-tabs">
    <button type="button" class="tab-btn" class:active={activeTab === 'session'} onclick={() => onSelectTab('session')}>
      Session{#if artifactCount > 0} <span class="tab-count">{artifactCount}</span>{/if}
    </button>
    <button type="button" class="tab-btn" class:active={activeTab === 'workspace'} onclick={() => onSelectTab('workspace')}>
      Workspace
    </button>
  </div>
  <button type="button" class="artifact-close" onclick={onClose}>&times;</button>
</div>

<style>
  .artifact-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .artifact-title {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
  }

  .artifact-tabs {
    display: flex;
    gap: 1px;
    margin-left: var(--space-2);
  }

  .tab-btn {
    background: none;
    border: 1px solid var(--border-subtle);
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
    padding: 2px 8px;
    border-radius: var(--radius-sm);
    transition: all var(--duration-fast);
  }
  .tab-btn:hover { color: var(--text-primary); border-color: var(--border-default); }
  .tab-btn.active { color: var(--primary); border-color: var(--primary); background: rgba(224, 145, 69, 0.08); }

  .tab-count {
    font-size: 9px;
    background: var(--surface-elevated);
    padding: 0 4px;
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
</style>
