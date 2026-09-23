<script lang="ts">
  // Status bar under the chat composer (#968): tier, permission mode, active
  // cwd, and this session's cost. Everything reads from the session store;
  // the cwd switch is delegated to Chat, which owns its feedback.
  import { onMount } from 'svelte'
  import { t } from '../i18n'
  import { shortCwdLabel } from '../lib/sessionLabels'
  import { isPinnableTier, pinnableTiers } from '../lib/tierRecommendation'
  import { chatSession } from '../lib/stores/chatSession'
  import type { ChatTier } from '../lib/types'

  interface Props {
    // Resolves true when the switch succeeded.
    onCwdSelect: (path: string) => Promise<boolean>
  }

  let { onCwdSelect }: Props = $props()

  let cwdState = $derived(chatSession.cwd)
  let cwdBusy = $derived(chatSession.cwdBusy)
  let cwdDropdownOpen = $state(false)

  let pinnedTier = $derived(chatSession.pinnedTier)
  let tierOptions = $derived(chatSession.tierOptions)
  let lastTier = $derived(chatSession.contextInfo.llm_tier ?? '')
  let lastModel = $derived(chatSession.contextInfo.llm_model ?? '')
  // Tiers the server cannot take per turn yet (anything but heavy/standard/light).
  let customTiers = $derived(tierOptions.filter((option) => !isPinnableTier(option.name)))

  // The tier the next turn will use, if we know it: the pin, else the last turn's.
  let effectiveTier = $derived(pinnedTier ?? lastTier)
  let effectiveKind = $derived(tierOptions.find((option) => option.name === effectiveTier)?.kind ?? '')
  // Permission mode only means something for the Claude Code CLI provider.
  let showPermission = $derived(effectiveKind === 'claude-code-cli')
  let permissionMode = $derived(chatSession.permissionModeOverride || $t.statusBar.permissionDefault)

  let usage = $derived(chatSession.usage)

  onMount(() => {
    void chatSession.loadTierOptions()
  })

  $effect(() => {
    void chatSession.activeSessionId
    cwdDropdownOpen = false
  })

  function tierLabel(tier: ChatTier): string {
    const model = tierOptions.find((option) => option.name === tier)?.model
    return model ? `${tier} · ${model}` : tier
  }

  function selectTier(value: string) {
    chatSession.setPinnedTier(isPinnableTier(value) ? value : null)
  }

  async function transitionCwd(path: string) {
    if (await onCwdSelect(path)) cwdDropdownOpen = false
  }

  function formatCost(usd: number): string {
    if (usd === 0) return '$0'
    return usd < 0.01 ? `$${usd.toFixed(4)}` : `$${usd.toFixed(2)}`
  }

  function formatTokens(count: number): string {
    if (count >= 1_000_000) return `${(count / 1_000_000).toFixed(1)}M`
    if (count >= 1_000) return `${(count / 1_000).toFixed(1)}k`
    return String(count)
  }
</script>

<div class="status-bar" role="group" aria-label={$t.statusBar.label}>
  <label class="status-item tier-picker" title={pinnedTier ? $t.statusBar.tierPinnedHint(pinnedTier) : $t.statusBar.tierAutoHint}>
    <span class="status-label">{$t.statusBar.tier}</span>
    <select
      class="tier-select"
      class:pinned={!!pinnedTier}
      value={pinnedTier ?? 'auto'}
      onchange={(event) => selectTier((event.currentTarget as HTMLSelectElement).value)}
    >
      <option value="auto">{$t.statusBar.tierAuto}</option>
      {#each pinnableTiers as tier (tier)}
        <option value={tier}>{tierLabel(tier)}</option>
      {/each}
      {#each customTiers as option (option.name)}
        <option value={option.name} disabled title={$t.statusBar.customTiersHint}>{option.name}</option>
      {/each}
    </select>
  </label>

  {#if lastTier}
    <span class="status-item status-muted" data-testid="status-served-by">{$t.statusBar.servedBy(lastTier, lastModel)}</span>
  {/if}

  {#if showPermission}
    <span class="status-item permission-chip" title={$t.statusBar.permissionHint} aria-disabled="true">
      <span class="status-label">{$t.statusBar.permission}</span>
      <strong>{permissionMode}</strong>
    </span>
  {/if}

  <span class="status-spacer"></span>

  {#if cwdState}
    <div class="cwd-hud">
      <button
        type="button"
        class="cwd-chip"
        title={cwdState.current}
        disabled={cwdBusy}
        onclick={() => { cwdDropdownOpen = !cwdDropdownOpen }}
      >
        <span class="cwd-chip-label">cwd</span>
        <strong>{shortCwdLabel(cwdState.current)}</strong>
      </button>
      {#if cwdDropdownOpen}
        <div class="cwd-dropdown" role="menu">
          {#each cwdState.eligible as path (path)}
            <button
              type="button"
              class="cwd-dropdown-item"
              class:active={path === cwdState.current}
              disabled={cwdBusy}
              title={path}
              onclick={() => transitionCwd(path)}
            >
              {shortCwdLabel(path)}
              {#if path === cwdState.current}<span class="cwd-active-marker">●</span>{/if}
            </button>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  {#if usage}
    <span class="status-item status-cost" title={$t.statusBar.costHint} data-testid="status-cost">
      <strong>{formatCost(usage.costUSD)}</strong>
      <span class="status-muted">{$t.statusBar.tokens(formatTokens(usage.inputTokens + usage.outputTokens))}</span>
    </span>
  {/if}
</div>

<style>
  /* Wraps rather than clips: the chat column narrows when docks open. */
  .status-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
    min-height: 28px;
    margin-top: var(--space-2);
    padding: 0 var(--space-1);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .status-item {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    white-space: nowrap;
  }

  .status-label {
    color: var(--text-tertiary);
    letter-spacing: 0.06em;
    text-transform: uppercase;
    font-size: 0.6875rem;
  }

  .status-muted {
    color: var(--text-tertiary);
  }

  .status-spacer {
    flex: 1;
  }

  .tier-select {
    padding: 2px var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    cursor: pointer;
  }

  .tier-select:focus-visible {
    outline: none;
    border-color: var(--primary);
  }

  .tier-select.pinned {
    border-color: rgba(var(--primary-rgb), 0.45);
    color: var(--primary-text);
  }

  .permission-chip {
    padding: 2px var(--space-2);
    border: 1px dashed var(--border-default);
    border-radius: var(--radius-sm);
    cursor: help;
  }

  .permission-chip strong {
    color: var(--text-secondary);
    font-weight: 500;
  }

  .status-cost strong {
    color: var(--text-primary);
    font-weight: 500;
  }

  @media (max-width: 900px) {
    .status-spacer {
      display: none;
    }
  }

  /* Active-cwd HUD. The dropdown is absolutely positioned and opens
     upward, since the bar sits at the bottom of the chat column. */
  .cwd-hud {
    position: relative;
    display: inline-flex;
    /* Shrinks before the session actions do; the path truncates inside. */
    flex: 0 1 auto;
    min-width: 0;
  }

  .cwd-chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    min-width: 0;
    max-width: min(240px, 100%);
    overflow: hidden;
    padding: 3px var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-base);
    color: var(--text-secondary);
    cursor: pointer;
    font-size: var(--text-xs);
    font-family: var(--font-mono);
    transition:
      background var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out);
  }

  .cwd-chip:hover:not(:disabled) {
    border-color: var(--primary);
    background: var(--surface-elevated);
  }

  .cwd-chip:disabled {
    cursor: progress;
    opacity: 0.7;
  }

  .cwd-chip-label {
    color: var(--text-tertiary);
    font-family: var(--font-display);
  }

  .cwd-chip strong {
    min-width: 0;
    overflow: hidden;
    color: var(--primary);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cwd-dropdown {
    position: absolute;
    bottom: calc(100% + 4px);
    right: 0;
    z-index: 50;
    display: flex;
    flex-direction: column;
    min-width: 220px;
    max-width: 360px;
    padding: var(--space-1);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    box-shadow: var(--shadow-md, 0 4px 12px rgba(0, 0, 0, 0.25));
  }

  .cwd-dropdown-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-primary);
    cursor: pointer;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    text-align: left;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cwd-dropdown-item:hover:not(:disabled) {
    background: var(--surface-base);
  }

  .cwd-dropdown-item.active {
    color: var(--primary);
  }

  .cwd-active-marker {
    margin-left: auto;
    color: var(--primary);
    font-size: var(--text-xs);
  }
</style>
