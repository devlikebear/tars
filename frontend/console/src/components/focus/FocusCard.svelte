<script lang="ts">
  // One card of the deck (ADR §7) and its actions: a gate (G1 plan; later
  // gates approve / request changes / stop), a decision (option buttons or a
  // free answer), a finding (fix / dismiss), and the informational cards —
  // verification failure, report, change, notice — which are acknowledged.
  import { t } from '../../i18n'
  import type {
    FocusCard,
    FocusChangePayload,
    FocusDecision,
    FocusFinding,
    FocusGateAction,
    FocusPlan,
    FocusPRDraft,
    FocusReport,
  } from '../../lib/types'
  import FocusChangeCard from './FocusChangeCard.svelte'
  import FocusPlanGate from './FocusPlanGate.svelte'

  interface Props {
    card: FocusCard
    // The pipeline's open gate ('' when none).
    openGate: string
    busy: boolean
    onGate: (gate: string, action: FocusGateAction, note?: string, edits?: FocusPlan) => void
    onDecide: (card: FocusCard, decision: string) => void
  }

  let { card, openGate, busy, onGate, onDecide }: Props = $props()

  let answer = $state('')
  let asking = $state(false)
  let note = $state('')

  let decided = $derived(card.state === 'decided')
  // The open gate's card is the newest undecided gate card.
  let gateOpen = $derived(card.kind === 'gate' && !decided && !!openGate)
  let isPRDraft = $derived(card.kind === 'report' && isDraft(card.payload))

  function isDraft(payload: unknown): payload is FocusPRDraft {
    return !!payload && typeof payload === 'object' && 'body' in payload && !('summary' in payload)
  }

  function asPlan(payload: unknown): FocusPlan | null {
    return payload && typeof payload === 'object' && Array.isArray((payload as FocusPlan).tasks) ? (payload as FocusPlan) : null
  }

  function errorsOf(payload: unknown): string[] {
    const errors = (payload as { errors?: unknown } | null)?.errors
    return Array.isArray(errors) ? errors.filter((e): e is string => typeof e === 'string') : []
  }

  // Facts of a verification failure (P2): command, exit code, excerpt.
  function failureFacts(payload: unknown): { command: string; exit: string; excerpt: string } {
    const p = (payload ?? {}) as Record<string, unknown>
    return {
      command: typeof p.command === 'string' ? p.command : '',
      exit: p.exit_code !== undefined ? String(p.exit_code) : '',
      excerpt: typeof p.excerpt === 'string' ? p.excerpt : typeof p.output === 'string' ? p.output : '',
    }
  }

  function sendAnswer() {
    const text = answer.trim()
    if (!text) return
    onDecide(card, text)
    answer = ''
  }

  function decisionLabel(decision: string | undefined): string {
    switch (decision) {
      case 'approve':
        return $t.focus.gate.approve
      case 'request_changes':
        return $t.focus.gate.requestChanges
      case 'stop':
        return $t.focus.gate.stop
      case 'acknowledged':
        return $t.focus.card.acknowledged
      case 'fix':
        return $t.focus.finding.fix
      case 'dismiss':
        return $t.focus.finding.dismiss
      default:
        return decision ?? ''
    }
  }
</script>

<article class="focus-card kind-{card.kind}" class:decided data-testid="focus-card" data-kind={card.kind} data-card-id={card.id}>
  <header class="card-head">
    <span class="badge kind-badge">{$t.focus.kinds[card.kind] ?? card.kind}</span>
    <h3 class="card-title" data-content>{card.kind === 'gate' && card.stage === 'plan' ? $t.focus.gate.planTitle : card.title}</h3>
    <span class="card-meta">
      {#if card.turn > 0}<span class="mono">{$t.focus.card.turn(card.turn)}</span>{/if}
      <span class="badge {card.state === 'unseen' ? 'badge-accent' : 'badge-default'}">{$t.focus.states[card.state]}</span>
    </span>
  </header>

  <div class="card-body">
    {#if card.kind === 'gate'}
      {#if card.stage === 'plan'}
        <FocusPlanGate
          cardId={card.id}
          plan={asPlan(card.payload)}
          open={gateOpen && openGate === 'plan'}
          {busy}
          onApprove={(edits) => onGate('plan', 'approve', undefined, edits)}
          onRequestChanges={(text) => onGate('plan', 'request_changes', text)}
          onStop={() => onGate('plan', 'stop')}
        />
      {:else if gateOpen}
        {#if asking}
          <textarea rows="3" bind:value={note} placeholder={$t.focus.gate.notePlaceholder} disabled={busy}></textarea>
          <div class="actions">
            <button type="button" class="btn btn-secondary btn-sm" disabled={busy || !note.trim()} onclick={() => onGate(openGate, 'request_changes', note.trim())}>{$t.focus.gate.sendChanges}</button>
            <button type="button" class="btn btn-ghost btn-sm" onclick={() => { asking = false }}>{$t.focus.gate.cancel}</button>
          </div>
        {:else}
          <div class="actions">
            <button type="button" class="btn btn-primary" disabled={busy} onclick={() => onGate(openGate, 'approve')}>{$t.focus.gate.approve}</button>
            <button type="button" class="btn btn-secondary" disabled={busy} onclick={() => { asking = true }}>{$t.focus.gate.requestChanges}</button>
            <button type="button" class="btn btn-danger" disabled={busy} onclick={() => onGate(openGate, 'stop')}>{$t.focus.gate.stop}</button>
          </div>
        {/if}
      {/if}
    {:else if card.kind === 'decision'}
      {@const d = card.payload as FocusDecision | undefined}
      <div class="options" role="group" aria-label={$t.focus.decision.answer}>
        {#each d?.options ?? [] as option, i (i)}
          <button
            type="button"
            class="btn option"
            class:btn-primary={decided && card.decision === option}
            class:btn-secondary={!(decided && card.decision === option)}
            disabled={busy || decided}
            data-testid="focus-decision-option"
            onclick={() => onDecide(card, option)}
          >
            {#if i < 9}<kbd class="mono">{i + 1}</kbd>{/if}
            <span data-content>{option}</span>
          </button>
        {/each}
      </div>
      {#if !decided}
        <div class="answer">
          <input type="text" bind:value={answer} placeholder={$t.focus.decision.answerPlaceholder} disabled={busy} aria-label={$t.focus.decision.answer} onkeydown={(e) => { if (e.key === 'Enter' && !e.isComposing) sendAnswer() }} />
          <button type="button" class="btn btn-secondary btn-sm" disabled={busy || !answer.trim()} onclick={sendAnswer}>{$t.focus.decision.send}</button>
        </div>
      {/if}
    {:else if card.kind === 'finding'}
      {@const f = card.payload as FocusFinding | undefined}
      <p class="finding-loc">
        <span class="badge {f?.severity === 'high' ? 'badge-error' : f?.severity === 'medium' ? 'badge-warning' : 'badge-default'}">{f?.severity ?? ''}</span>
        {#if f?.file}<span class="mono" data-content>{f.file}{f.line ? `:${f.line}` : ''}</span>{/if}
      </p>
      {#if f?.scenario}
        <h4 class="label">{$t.focus.finding.scenario}</h4>
        <p class="prose" data-content>{f.scenario}</p>
      {/if}
      {#if !decided}
        <div class="actions">
          <button type="button" class="btn btn-secondary btn-sm" disabled={busy} onclick={() => onDecide(card, 'fix')}>{$t.focus.finding.fix}</button>
          <button type="button" class="btn btn-ghost btn-sm" disabled={busy} onclick={() => onDecide(card, 'dismiss')}>{$t.focus.finding.dismiss}</button>
        </div>
      {/if}
    {:else if card.kind === 'failure'}
      {@const f = failureFacts(card.payload)}
      {#if f.command}<p class="mono" data-content>$ {f.command}{f.exit ? ` → ${f.exit}` : ''}</p>{/if}
      {#if f.excerpt}<pre class="mono" data-content>{f.excerpt}</pre>{/if}
    {:else if card.kind === 'report' && isPRDraft}
      {@const pr = card.payload as FocusPRDraft}
      <h4 class="label">{$t.focus.report.prTitle}</h4>
      <p class="prose" data-content>{pr.title}</p>
      <h4 class="label">{$t.focus.report.prBody}</h4>
      <pre class="prose-pre" data-content>{pr.body}</pre>
    {:else if card.kind === 'report'}
      {@const r = card.payload as FocusReport | undefined}
      <!-- The title is the summary's first line; show the rest only. -->
      {#if r?.summary && r.summary.trim() !== card.title}
        <p class="prose summary" data-content>{r.summary}</p>
      {/if}
      {#if r?.risks?.length}
        <h4 class="label">{$t.focus.report.risks}</h4>
        <ul class="risks">
          {#each r.risks as risk, i (i)}<li data-content>{risk}</li>{/each}
        </ul>
      {/if}
      {#if r?.decisions?.length}
        <p class="muted">{$t.focus.report.decisions(r.decisions.length)}</p>
      {/if}
    {:else if card.kind === 'change'}
      <FocusChangeCard change={card.payload as FocusChangePayload} />
    {:else if card.kind === 'notice'}
      {@const errors = errorsOf(card.payload)}
      {#if errors.length}
        <h4 class="label">{$t.focus.notice.errors}</h4>
        <ul class="risks">
          {#each errors as error, i (i)}<li class="mono" data-content>{error}</li>{/each}
        </ul>
      {/if}
    {/if}

    {#if (card.kind === 'report' || card.kind === 'change' || card.kind === 'failure' || card.kind === 'notice') && !decided}
      <div class="actions">
        <button type="button" class="btn btn-secondary btn-sm" disabled={busy} data-testid="focus-acknowledge" onclick={() => onDecide(card, 'acknowledged')}>{$t.focus.card.acknowledge}</button>
      </div>
    {/if}

    {#if decided && card.decision && card.kind !== 'decision'}
      <p class="decided-line">{$t.focus.card.decided(decisionLabel(card.decision))}</p>
    {/if}
  </div>
</article>

<style>
  .focus-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-5);
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
  }

  .kind-gate:not(.decided),
  .kind-decision:not(.decided),
  .kind-finding:not(.decided) {
    border-color: rgba(var(--primary-rgb), 0.45);
  }

  .kind-failure:not(.decided),
  .kind-notice:not(.decided) {
    border-color: var(--warning);
  }

  .card-head {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .card-title {
    flex: 1 1 auto;
    margin: 0;
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-primary);
  }

  .card-meta {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .kind-badge {
    background: var(--surface-elevated);
    color: var(--text-secondary);
  }

  .card-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  h4 {
    margin: 0;
  }

  .prose {
    margin: 0;
    color: var(--text-primary);
    line-height: var(--leading-normal);
    white-space: pre-wrap;
  }

  .prose-pre,
  pre {
    margin: 0;
    padding: var(--space-2);
    background: var(--surface-inset);
    border-radius: var(--radius-sm);
    font-size: var(--text-sm);
    white-space: pre-wrap;
    max-height: 320px;
    overflow: auto;
  }

  .risks {
    margin: 0;
    padding-left: var(--space-5);
    color: var(--text-secondary);
  }

  .muted,
  .decided-line {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .options {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .option {
    justify-content: flex-start;
    gap: var(--space-2);
    text-align: left;
  }

  kbd {
    padding: 0 var(--space-1);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
  }

  .answer {
    display: flex;
    gap: var(--space-2);
  }

  .answer input,
  textarea {
    flex: 1;
    padding: var(--space-2);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  .answer input:focus,
  textarea:focus {
    outline: none;
    border-color: var(--primary);
  }

  .finding-loc {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>
