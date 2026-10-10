<script lang="ts">
  // The developer's own finding (#1196): what they found reading the diff
  // goes in as a finding card of the review triage or the pr_review fix
  // round, so it reaches the fix turn without a detour through a decision
  // card's answer. Collapsed to one ghost button until asked for.
  import { t } from '../../i18n'
  import type { FocusFindingInput } from '../../lib/types'

  interface Props {
    // Where the card goes, for the hint line.
    kind: 'review' | 'pr_review'
    busy?: boolean
    // Resolves true when the finding was added; the form then clears.
    onAdd: (finding: FocusFindingInput) => Promise<boolean>
  }

  let { kind, busy = false, onAdd }: Props = $props()

  let open = $state(false)
  let title = $state('')
  let scenario = $state('')
  let file = $state('')
  let line = $state('')
  let severity = $state('medium')
  let titleBox = $state<HTMLInputElement | null>(null)

  function show() {
    open = true
    requestAnimationFrame(() => titleBox?.focus())
  }

  async function add(decision?: 'fix') {
    const text = title.trim()
    if (!text || busy) return
    const finding: FocusFindingInput = { title: text, severity }
    if (scenario.trim()) finding.scenario = scenario.trim()
    if (file.trim()) finding.file = file.trim()
    const n = Number.parseInt(line, 10)
    if (finding.file && n > 0) finding.line = n
    if (decision) finding.decision = decision
    if (!(await onAdd(finding))) return
    title = scenario = file = line = ''
    severity = 'medium'
    open = false
  }
</script>

{#if !open}
  <div class="add-row">
    <button type="button" class="btn btn-ghost btn-sm" onclick={show} data-testid="focus-add-finding">+ {$t.focus.addFinding.open}</button>
  </div>
{:else}
  <form class="add-finding" onsubmit={(e) => { e.preventDefault(); void add('fix') }} data-testid="focus-add-finding-form">
    <label class="label" for="focus-add-finding-title">{$t.focus.addFinding.label}</label>
    <input
      id="focus-add-finding-title"
      type="text"
      bind:this={titleBox}
      bind:value={title}
      maxlength="200"
      placeholder={$t.focus.addFinding.titlePlaceholder}
      disabled={busy}
      data-testid="focus-add-finding-title"
    />
    <textarea
      rows="2"
      bind:value={scenario}
      placeholder={$t.focus.addFinding.scenarioPlaceholder}
      aria-label={$t.focus.finding.scenario}
      disabled={busy}
      data-testid="focus-add-finding-scenario"
    ></textarea>
    <div class="where">
      <input type="text" class="mono file" bind:value={file} placeholder={$t.focus.addFinding.filePlaceholder} aria-label={$t.focus.addFinding.file} disabled={busy} data-testid="focus-add-finding-file" />
      <input type="number" class="mono line" min="1" bind:value={line} placeholder={$t.focus.addFinding.linePlaceholder} aria-label={$t.focus.addFinding.line} disabled={busy} data-testid="focus-add-finding-line" />
      <select bind:value={severity} aria-label={$t.focus.addFinding.severity} disabled={busy} data-testid="focus-add-finding-severity">
        {#each ['high', 'medium', 'low'] as level (level)}
          <option value={level}>{$t.focus.finding.severity[level]}</option>
        {/each}
      </select>
    </div>
    <div class="actions">
      <button type="submit" class="btn btn-primary btn-sm" disabled={busy || !title.trim()} data-testid="focus-add-finding-fix">{$t.focus.addFinding.addFix}</button>
      <button type="button" class="btn btn-secondary btn-sm" disabled={busy || !title.trim()} onclick={() => void add()} data-testid="focus-add-finding-later">{$t.focus.addFinding.addLater}</button>
      <button type="button" class="btn btn-ghost btn-sm" onclick={() => { open = false }}>{$t.focus.addFinding.cancel}</button>
    </div>
    <p class="hint">{kind === 'review' ? $t.focus.addFinding.hintReview : $t.focus.addFinding.hintPR}</p>
  </form>
{/if}

<style>
  .add-row {
    display: flex;
  }

  .add-finding {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
  }

  input,
  textarea,
  select {
    box-sizing: border-box;
    min-width: 0;
    padding: var(--space-2);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  textarea {
    resize: vertical;
  }

  input:focus,
  textarea:focus,
  select:focus {
    outline: none;
    border-color: var(--primary);
  }

  .where {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .file {
    flex: 1 1 220px;
  }

  .line {
    flex: 0 0 96px;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .hint {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }
</style>
