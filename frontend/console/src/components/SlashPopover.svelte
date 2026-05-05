<script lang="ts">
  import type { SlashCommandCandidate } from '../lib/slash'

  interface Props {
    candidates: SlashCommandCandidate[]
    activeIndex: number
    onSelect: (candidate: SlashCommandCandidate) => void
    onExecute: (candidate: SlashCommandCandidate) => void
  }

  let { candidates, activeIndex, onSelect, onExecute }: Props = $props()

  function handleClick(candidate: SlashCommandCandidate) {
    if (candidate.kind === 'builtin') {
      onExecute(candidate)
      return
    }
    onSelect(candidate)
  }

  function sectionLabel(kind: SlashCommandCandidate['kind']): string {
    if (kind === 'builtin') return 'Built-in'
    if (kind === 'command') return 'Commands'
    return 'Skills'
  }

  function kindLabel(kind: SlashCommandCandidate['kind']): string {
    if (kind === 'skill') return 'SKILL'
    if (kind === 'command') return 'CMD'
    return 'CMD'
  }

  function sourceLabel(candidate: SlashCommandCandidate): string {
    if (candidate.kind === 'builtin') return 'built-in'
    if (candidate.kind === 'command') return candidate.source || 'command'
    return candidate.source || 'skill'
  }
</script>

<div class="slash-popover" role="listbox" aria-label="Slash command suggestions">
  {#each candidates as candidate, i}
    {#if i === 0 || candidates[i - 1]?.kind !== candidate.kind}
      <div class="slash-section">{sectionLabel(candidate.kind)}</div>
    {/if}
    <button
      type="button"
      role="option"
      aria-selected={i === activeIndex}
      class:active={i === activeIndex}
      class="slash-option"
      onmousedown={(e) => e.preventDefault()}
      onclick={() => handleClick(candidate)}
    >
      <span class="slash-kind">{kindLabel(candidate.kind)}</span>
      <span class="slash-main">
        /{candidate.command}
        {#if candidate.aliasOf}<span class="slash-alias">/{candidate.aliasOf}</span>{/if}
      </span>
      <span class="slash-source">{sourceLabel(candidate)}</span>
      <span class="slash-description">{candidate.description}</span>
    </button>
  {/each}
</div>

<style>
  .slash-popover {
    position: absolute;
    left: 0;
    right: 0;
    bottom: calc(100% + 6px);
    z-index: 20;
    max-height: 300px;
    overflow-y: auto;
    padding: 4px;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    box-shadow: var(--shadow-lg);
  }

  .slash-section {
    padding: 5px 8px 3px;
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    text-transform: uppercase;
  }

  .slash-option {
    display: grid;
    grid-template-columns: 52px minmax(0, 1fr) auto;
    grid-template-rows: auto auto;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    min-height: 34px;
    padding: 4px 8px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    text-align: left;
  }

  .slash-option:hover,
  .slash-option.active {
    background: rgba(224, 145, 69, 0.12);
    color: var(--text-primary);
  }

  .slash-kind,
  .slash-source {
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
  }

  .slash-source {
    max-width: 220px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .slash-main {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .slash-alias {
    margin-left: var(--space-2);
    color: var(--text-ghost);
    font-size: 10px;
  }

  .slash-description {
    grid-column: 2 / 4;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-ghost);
    font-size: 10px;
  }
</style>
